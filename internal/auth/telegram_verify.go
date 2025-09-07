package auth

import (
	"encoding/json"
	"log"
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

	// ДОБАВИТЬ ЭТУ ПРОВЕРКУ:
	user, err := h.db.GetUserByPhoneHash(req.PhoneHash)
	if err != nil || user == nil {
		log.Printf("DEBUG: User not found for phone_hash: %s", req.PhoneHash)
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	log.Printf("DEBUG: User found - YUI: %s, saving OTP", user.YUI)

	// Если пользователь найден, сохраняем код
	h.auth.StoreOTP(req.PhoneHash, req.Code)
	h.db.SaveOTP(req.PhoneHash, req.Code, req.TelegramID)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
