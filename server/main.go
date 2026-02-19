package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"

	"github.com/CoupDeGrace92/CacheMaster/server/database"
)

type apiConfig struct {
	db     *database.Queries
	Secret string
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Problem loading env variables:", err)
		return
	}

	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Println("Problem loading db:", err)
		return
	}

	dbConfig := database.New(db)

	apiCfg := apiConfig{
		db:     dbConfig,
		Secret: os.Getenv("JWT_SECRET"),
	}

	ServerMux := http.NewServeMux()

	//User management endpoints
	ServerMux.Handle("POST /api/register", http.HandlerFunc(apiCfg.HandleNewUser))
	ServerMux.Handle("POST /api/login", http.HandlerFunc(apiCfg.HandleLogin))
	ServerMux.Handle("DELETE /api/delete_user", http.HandlerFunc(apiCfg.HandleDeleteUser))

	//Admin endpoints
	ServerMux.Handle("DELETE /admin/reset/users", http.HandlerFunc(apiCfg.HandleDeleteUsers))
	ServerMux.Handle("DELETE /admin/reset/data", http.HandlerFunc(apiCfg.HandleDeleteDataDB))
	/*
		//Data retrieval endpoints
		ServerMux.Handle("GET /api/data/{id}", http.HandlerFunc(apiCfg.HandleGetData))
		ServerMux.Handle("POST /api/data/{id}", http.HandlerFunc(apiCfg.HandleUpdateData)) //No idempotency in data
		ServerMux.Handle("POST /api/data/new", http.HandlerFunc(apiCfg.HandeNewData))
		ServerMux.Handle("DELETE /api/data/{id}", http.HandlerFunc(apiCfg.HandleDeleteData))
		ServerMux.Handle("GET /api/user_data/{username}", http.HandlerFunc(apiCfg.HandleGetDataByUser))
	*/
	server := &http.Server{
		Handler: ServerMux,
		Addr:    ":8080",
	}

	err = server.ListenAndServe()
	if err != nil {
		log.Println(err)
	}
}
