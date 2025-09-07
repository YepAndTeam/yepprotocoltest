package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"yep-protocol/internal/storage"
)

type TelegramVerifyHandler struct {
	db   *storage.DB
	auth *Service
}

func NewTelegramVerifyHandler(db *storage.DB, auth *Service) *TelegramVerifyHandler {
	return &TelegramVerifyHandler{
		db:   db,
		auth: auth,
	}
}

type TelegramVerification struct {
	PhoneHash string `json:"phone_hash"`
}

func (h *TelegramVerifyHandler) HandleTelegramCheck(w http.ResponseWriter, r *http.Request) {
	var req TelegramVerification
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	user, err := h.db.GetUserByPhoneHash(req.PhoneHash)
	if err == nil && user != nil {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"exists": true,
			"yui":    user.YUI,
		}); err != nil {
			fmt.Printf("DEBUG: Response encode error: %v\n", err)
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (h *TelegramVerifyHandler) HandleSaveCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PhoneHash  string `json:"phone_hash"`
		Code       string `json:"code"`
		TelegramID int64  `json:"telegram_id"`
	}

	// Debug: Read and log raw request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("DEBUG: Failed to read request body: %v\n", err)
		http.Error(w, "failed to read request", http.StatusBadRequest)
		return
	}
	fmt.Printf("DEBUG: Raw request body: %s\n", string(body))
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Decode JSON request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fmt.Printf("DEBUG: JSON decode error: %v\n", err)
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	fmt.Printf("DEBUG: Decoded - phone_hash: %s, code: %s, telegram_id: %d\n",
		req.PhoneHash, req.Code, req.TelegramID)

	// Check if user exists
	user, err := h.db.GetUserByPhoneHash(req.PhoneHash)
	if err != nil {
		fmt.Printf("DEBUG: User not found for phone_hash: %s\n", req.PhoneHash)
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	fmt.Printf("DEBUG: User found - YUI: %s\n", user.YUI)

	// Store OTP in auth service and database
	h.auth.StoreOTP(req.PhoneHash, req.Code)
	h.db.SaveOTP(req.PhoneHash, req.Code, req.TelegramID)

	// Respond with success
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		fmt.Printf("DEBUG: Response encode error: %v\n", err)
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}
}
