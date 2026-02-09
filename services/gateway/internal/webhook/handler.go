package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	commonv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/common/v1"
	ingestionv1 "github.com/ziyixi/SecondBrain/services/gateway/pkg/gen/ingestion/v1"
	"github.com/ziyixi/SecondBrain/services/gateway/internal/normalizer"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Handler processes incoming webhooks from external services.
type Handler struct {
	logger      *slog.Logger
	normalizer  *normalizer.Normalizer
	secret      string
	itemChan    chan *ingestionv1.InboxItem
}

// NewHandler creates a new webhook handler.
func NewHandler(logger *slog.Logger, secret string) *Handler {
	return &Handler{
		logger:     logger,
		normalizer: normalizer.New(),
		secret:     secret,
		itemChan:   make(chan *ingestionv1.InboxItem, 100),
	}
}

// Items returns the channel of incoming inbox items.
func (h *Handler) Items() <-chan *ingestionv1.InboxItem {
	return h.itemChan
}

// RegisterRoutes sets up HTTP routes for webhook endpoints.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /webhooks/email", h.handleEmail)
	mux.HandleFunc("POST /webhooks/slack", h.handleSlack)
	mux.HandleFunc("POST /webhooks/github", h.handleGitHub)
	mux.HandleFunc("POST /webhooks/generic", h.handleGeneric)
	mux.HandleFunc("GET /health", h.handleHealth)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
}

func (h *Handler) handleEmail(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Subject string `json:"subject"`
		Body    string `json:"body"`
		From    string `json:"from"`
		IsHTML  bool   `json:"is_html"`
	}

	if err := h.decodeBody(r, &payload); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	content, metadata := h.normalizer.NormalizeEmail(payload.Subject, payload.Body, payload.IsHTML)
	metadata["from"] = payload.From

	item := h.createInboxItem(content, "email", metadata)
	h.enqueueItem(item)

	h.successResponse(w, item.Id)
}

func (h *Handler) handleSlack(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Text    string `json:"text"`
		Channel string `json:"channel"`
		User    string `json:"user"`
	}

	if err := h.decodeBody(r, &payload); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	content, metadata := h.normalizer.NormalizeSlackMessage(payload.Text, payload.Channel, payload.User)
	item := h.createInboxItem(content, "slack", metadata)
	h.enqueueItem(item)

	h.successResponse(w, item.Id)
}

func (h *Handler) handleGitHub(w http.ResponseWriter, r *http.Request) {
	// Verify webhook signature if secret is configured
	if h.secret != "" {
		if !h.verifyGitHubSignature(r) {
			h.errorResponse(w, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		h.errorResponse(w, http.StatusBadRequest, "missing X-GitHub-Event header")
		return
	}

	var payload map[string]interface{}
	if err := h.decodeBody(r, &payload); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	content, metadata := h.normalizer.NormalizeGitHubWebhook(eventType, payload)
	item := h.createInboxItem(content, "github", metadata)
	h.enqueueItem(item)

	h.successResponse(w, item.Id)
}

func (h *Handler) handleGeneric(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Content  string            `json:"content"`
		Source   string            `json:"source"`
		Metadata map[string]string `json:"metadata"`
	}

	if err := h.decodeBody(r, &payload); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "invalid payload: "+err.Error())
		return
	}

	source := payload.Source
	if source == "" {
		source = "generic"
	}

	item := h.createInboxItem(payload.Content, source, payload.Metadata)
	h.enqueueItem(item)

	h.successResponse(w, item.Id)
}

func (h *Handler) createInboxItem(content, source string, metadata map[string]string) *ingestionv1.InboxItem {
	return &ingestionv1.InboxItem{
		Id:          uuid.New().String(),
		Content:     content,
		Source:      source,
		ReceivedAt:  timestamppb.New(time.Now()),
		RawMetadata: metadata,
		Priority:    commonv1.Priority_PRIORITY_NORMAL,
		ContentType: "text/plain",
	}
}

func (h *Handler) enqueueItem(item *ingestionv1.InboxItem) {
	select {
	case h.itemChan <- item:
		h.logger.Info("item enqueued", "id", item.Id, "source", item.Source)
	default:
		h.logger.Warn("item channel full, dropping item", "id", item.Id)
	}
}

func (h *Handler) decodeBody(r *http.Request, v interface{}) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}
	defer r.Body.Close()

	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decoding JSON: %w", err)
	}
	return nil
}

func (h *Handler) verifyGitHubSignature(r *http.Request) bool {
	signature := r.Header.Get("X-Hub-Signature-256")
	if signature == "" {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return false
	}
	// Reset body for later reading
	r.Body = io.NopCloser(io.LimitReader(io.NopCloser(
		io.MultiReader(io.NopCloser(
			io.LimitReader(io.NopCloser(nil), 0)),
		)),
		0))

	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}

func (h *Handler) errorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message}) //nolint:errcheck
}

func (h *Handler) successResponse(w http.ResponseWriter, itemID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"item_id": itemID,
		"status":  "accepted",
	})
}
