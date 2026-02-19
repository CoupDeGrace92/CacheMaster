package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/CoupDeGrace92/CacheMaster/auth"
	"github.com/CoupDeGrace92/CacheMaster/server/database"
)

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Username  string    `json:"username"`
	Refresh   string    `json:"refresh_token"`
	Token     string    `json:"token"`
	Password  string    `json:"password"`
}

func (cfg *apiConfig) HandleNewUser(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	decoder := json.NewDecoder(r.Body)
	var rawParams parameters

	err := decoder.Decode(&rawParams)
	if err != nil {
		log.Println("Error decoding params:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	hash, err := auth.HashPassword(rawParams.Password)
	if err != nil {
		log.Println("Error hashing password:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if rawParams.Username == "" {
		log.Printf("Error: did not recieve a username\n")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userParams := database.CreateUserParams{
		Username: rawParams.Username,
		PassHash: hash,
	}

	user, err := cfg.db.CreateUser(r.Context(), userParams)
	if err != nil {
		log.Println("Error creating user", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	resp := User{
		ID:        user.ID,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
		Username:  user.Username,
	}

	dat, err := json.Marshal(resp)
	if err != nil {
		log.Println("Error marshalling json for response:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Contetent-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	w.Write(dat)
}

func (cfg *apiConfig) HandleLogin(w http.ResponseWriter, r *http.Request) {
	type Params struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Expires  int    `json:"expires_in"`
	}

	var params Params
	params.Expires = 3600

	err := json.NewDecoder(r.Body).Decode(params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("failed to decode json:", err)
		return
	}
	if params.Expires > 3600 {
		params.Expires = 3600
	}

	user, err := cfg.db.GetUser(r.Context(), params.Username)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Failed to fetch user:", err)
		return
	}
	authenticated, err := auth.CheckPasswordHash(params.Password, user.PassHash)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error authenticating user:", err)
		return
	}
	if !authenticated {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	token, err := auth.MakeJWT(user.ID, cfg.Secret, time.Duration(params.Expires))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error making jwt:", err)
		return
	}

	outUser := User{
		ID:       user.ID,
		Username: user.Username,
		Token:    token,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(outUser)
	if err != nil {
		log.Println("Error encoding into json:", err)
	}
}

func (cfg *apiConfig) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	type Params struct {
		Username string `json:"username"`
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error getting bearer token:", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.Secret)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Println("Error validating token:", err)
		return
	}

	var params Params

	err = json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error decoding request", err)
		w.Write([]byte("Need to include a username in json format to delete"))
		return
	}

	user, err := cfg.db.GetUser(r.Context(), params.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error fetching userid")
		return
	}

	if user.ID != userID {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	err = cfg.db.DeleteUser(r.Context(), user.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error deleting user", err)
		return
	}

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte("Successfully deleted user's data"))
}
