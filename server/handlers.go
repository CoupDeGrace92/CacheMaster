package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
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

func (cfg *apiConfig) HandleDeleteUsers(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("PLATFORM") != "dev" {
		w.WriteHeader(http.StatusUnauthorized)
		log.Println("Environment is not a dev environment - don't do this to prod you dunce")
		return
	}

	err := cfg.db.DeleteUsers(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error resetting db:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("users db reset"))
}

func (cfg *apiConfig) HandleDeleteDataDB(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("PLATFORM") != "dev" {
		w.WriteHeader(http.StatusUnauthorized)
		log.Println("Environment is not a dev environment - don't do this to prod you dunce")
		return
	}

	err := cfg.db.DeleteDataTable(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error resetting db:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("data db reset"))
}

func (cfg *apiConfig) HandleGetData(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error getting bearer token:", err)
		w.Write([]byte("Malformed or missing bearer token"))
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.Secret)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Println("Error authourizing user", err)
		return
	}

	type Params struct {
		Dataid uuid.UUID `json:"dataid"`
	}

	var params Params

	err = json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error decoding request", err)
		w.Write([]byte("Need to include data id to fetch"))
		return
	}

	dat, err := cfg.db.GetData(r.Context(), params.Dataid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error getting requested data", err)
		return
	}

	user, err := cfg.db.GetUser(r.Context(), dat.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error: failed to get user associated with data:", err)
		return
	}

	if user.ID != userID {
		w.WriteHeader(http.StatusUnauthorized)
		log.Println("Unauthorized user tried to access data")
		return
	}

	type Resp struct {
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Dat       string    `json:"dat"`
	}

	resp := Resp{
		CreatedAt: dat.CreatedAt,
		UpdatedAt: dat.UpdatedAt,
		Dat:       dat.Dat,
	}

	w.Header().Set("Content-Type", "json/application")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(resp)
	if err != nil {
		log.Println("Error encoding json response:", err)
	}
}

func (cfg *apiConfig) HandleGetDataByUser(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error getting bearer token", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.Secret)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type Params struct {
		Username string `json:"username"`
	}

	var params Params

	err = json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		log.Println("Error decoding params", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	user, err := cfg.db.GetUser(r.Context(), params.Username)
	if err != nil {
		log.Println("Error fetching user from db to get data entries", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if user.ID != userID {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	ids, err := cfg.db.GetDataByUser(r.Context(), params.Username)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		log.Println("Error fetching ids of data associated with user", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(ids)
	if err != nil {
		log.Println("Error encoding data list to json:", err)
	}
}

func (cfg *apiConfig) HandleUpdateData(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error getting bearer token:", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.Secret)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		log.Println("Error validating jwt", err)
		return
	}

	type Params struct {
		Username string `json:"username"`
		Dat      string `json:"dat"`
	}

	var params Params

	err = json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Error decoding json into struct:", err)
		return
	}

	user, err := cfg.db.GetUser(r.Context(), params.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Failed to fetch user from db:", err)
		return
	}

	if user.ID != userID {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	args := database.InsertDataParams{
		Dat:      params.Dat,
		Username: params.Username,
	}

	id, err := cfg.db.InsertData(r.Context(), args)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Failed to write to db:", err)
		w.Write([]byte("Failed to write to db"))
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "DATA ID: %s", id)
}

func (cfg *apiConfig) HandleNewData(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		log.Println("Failed to fetch bearer token:", err)
		w.Write([]byte("Failed to fetch bearer token"))
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.Secret)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type Params struct {
		Username string `json:"username"`
		Dat      string `json:"dat"`
	}

	var params Params

	err = json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Failed to decode response body into json:", err)
		return
	}
	user, err := cfg.db.GetUser(r.Context(), params.Username)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Failed to fetch user information from db:", err)
		return
	}

	if user.ID != userID {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	payload := database.InsertDataParams{
		Dat:      params.Dat,
		Username: user.Username,
	}

	_, err = cfg.db.InsertData(r.Context(), payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error inserting data into db:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Data added to db"))
}

func (cfg *apiConfig) HandleDeleteData(w http.ResponseWriter, r *http.Request) {
	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.Secret)
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	type Params struct {
		Username string    `json:"username"`
		DatID    uuid.UUID `json:"datid"`
	}

	var params Params

	err = json.NewDecoder(r.Body).Decode(&params)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error decoding request:", err)
		return
	}

	user, err := cfg.db.GetUser(r.Context(), params.Username)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		log.Println("Error getting user info:", err)
		return
	}

	if user.ID != userID {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	err = cfg.db.DeleteDataByID(r.Context(), params.DatID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		log.Println("Error could not delete data:", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Deleted requested data"))
}
