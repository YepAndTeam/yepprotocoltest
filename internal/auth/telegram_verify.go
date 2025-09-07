package auth

import (
	"encoding/json"
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
		json.NewEncoder(w).Encode(map[string]interface{}{
			"exists": true,
			"yui":    user.YUI,
		})
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

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	// Сохраняем код по phone_hash
	h.auth.StoreOTPByPhoneHash(req.PhoneHash, req.Code)

	// Также можно сохранить в БД для надёжности
	h.db.SaveOTP(req.PhoneHash, req.Code, req.TelegramID)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}
