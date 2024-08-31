package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PatriciaChebet/chirpy-latest-project/database"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

type CustomClaims struct {
	jwt.RegisteredClaims
}

type apiConfig struct {
	fileserveHits int
	DB            *database.DB
	JWT_SECRET    string
}

type Chirp struct {
	ID   int    `json:"id"`
	Body string `json:"body"`
}

type User struct {
	ID    int    `json:"id"`
	Email string `json:"email"`
	Token string `json:"token"`
}

type JwtToken struct {
	Issuer    string    `json:"issuer"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Subject   string    `json:"subject"`
}

func main() {
	const filepathRoot = "."
	const port = "8080"

	godotenv.Load()
	jwtSecret := os.Getenv("JWT_SECRET")

	db, err := database.NewDB("database.json")
	if err != nil {
		log.Fatal(err)
	}

	apiCfg := apiConfig{
		fileserveHits: 0,
		DB:            db,
		JWT_SECRET:    jwtSecret,
	}

	mux := http.NewServeMux()
	mux.Handle("/app/*", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))))
	mux.Handle("/admin/*", apiCfg.middlewareMetricsInc(http.StripPrefix("/admin", http.FileServer(http.Dir(filepathRoot)))))
	mux.HandleFunc("GET /api/healthz", calcHealthz)
	mux.HandleFunc("GET /admin/metrics", apiCfg.calcServerHits)
	mux.HandleFunc("GET /api/reset", apiCfg.resetServerHits)
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerChirpsCreate)
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerChirpsRetrieve)
	mux.HandleFunc("GET /api/chirps/{id}", apiCfg.handleChirpRetrieval)
	mux.HandleFunc("POST /api/users", apiCfg.handleUsersCreate)
	mux.HandleFunc("POST /api/login", apiCfg.loginUser)
	mux.HandleFunc("PUT /api/users", apiCfg.handleUsersUpdate)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Server started on port %s\n", port)
	log.Fatal(srv.ListenAndServe())
}

func calcHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) calcServerHits(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("<h1>Welcome, Chirpy Admin</h1>"))
	w.Write([]byte(fmt.Sprintf("Chirpy has been visited %d times!", cfg.fileserveHits)))
}

func (cfg *apiConfig) resetServerHits(w http.ResponseWriter, r *http.Request) {
	cfg.fileserveHits = 0
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset"))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserveHits++
		next.ServeHTTP(w, r)
	})
}

func validate_chirp(body string) (string, error) {
	const maxChirpLength = 140
	if len(body) > maxChirpLength {
		return "", errors.New("Chirp is too long")
	}

	badWords := map[string]struct{}{
		"kerfuffle": {},
		"sharbert":  {},
		"fornax":    {},
	}
	cleaned := getCleanedBody(body, badWords)
	return cleaned, nil
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't marshal response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	if code > 499 {
		log.Printf("Responding with error: %s", message)
	}

	type errorResponse struct {
		Error string `json:"error"`
	}

	respondWithJSON(w, code, errorResponse{Error: message})
}

func (cfg *apiConfig) handlerChirpsCreate(w http.ResponseWriter, r *http.Request) {
	type parameters struct {
		Body string `json:"body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := parameters{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	cleaned, err := validate_chirp(params.Body)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	chirp, err := cfg.DB.CreateChirp(cleaned)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create chirp")
		return
	}

	respondWithJSON(w, http.StatusCreated, Chirp{
		ID:   chirp.ID,
		Body: chirp.Body,
	})
}

func (cfg *apiConfig) handleUsersCreate(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	decoder := json.NewDecoder(r.Body)
	parameters := params{}
	err := decoder.Decode(&parameters)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	hashedPassword, err := HashPassword(parameters.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't hash the password")
	}

	user, err := cfg.DB.CreateUser(parameters.Email, hashedPassword)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create user")
		return
	}

	respondWithJSON(w, http.StatusCreated, User{
		ID:    user.ID,
		Email: user.Email,
	})
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func (cfg *apiConfig) loginUser(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Email            string `json:"email"`
		Password         string `json:"password"`
		ExpiresInSeconds int    `json:"expires_in_seconds"`
	}

	decoder := json.NewDecoder(r.Body)
	parameters := params{}
	err := decoder.Decode(&parameters)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	user, err := cfg.DB.FindUserByEmail(parameters.Email)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not find user with that email")
	}
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(parameters.Password))
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Passwords did not match")
	}

	var expiresIn int
	if parameters.ExpiresInSeconds != 0 {
		if parameters.ExpiresInSeconds < 86400 {
			expiresIn = parameters.ExpiresInSeconds
		}
		expiresIn = 86400
	} else {
		expiresIn = 86400 // Default value
	}

	claims := CustomClaims{
		jwt.RegisteredClaims{
			Issuer:    "chirpy",
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiresIn) * time.Second)),
			Subject:   strconv.Itoa(user.ID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	log.Printf("Expires at %s\n", claims.ExpiresAt)
	signedToken, err := token.SignedString([]byte(cfg.JWT_SECRET))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Token could not be signed")
	}

	respondWithJSON(w, http.StatusOK, User{
		ID:    user.ID,
		Email: user.Email,
		Token: signedToken,
	})
}

func (cfg *apiConfig) handleUsersUpdate(w http.ResponseWriter, r *http.Request) {
	type params struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	bearerToken := r.Header.Get("Authorization")
	log.Printf("Bearer token sent %s\n", bearerToken)
	trimmedToken := strings.TrimPrefix(bearerToken, "Bearer ")
	log.Printf("trimmedToken %s\n", trimmedToken)

	returnedToken, err := jwt.ParseWithClaims(trimmedToken, &CustomClaims{}, func(trimmedToken *jwt.Token) (interface{}, error) {
		return []byte(cfg.JWT_SECRET), nil
	})
	log.Printf("returnedToken %s\n", returnedToken)
	if err != nil {
		log.Printf("Error with Token %s\n", err)
		respondWithError(w, http.StatusUnauthorized, "Invalid token")
		return
	}

	tokenClaims, ok := returnedToken.Claims.(*CustomClaims)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "Couldn't parse token claims")
		return
	}
	tokenStruct := JwtToken{
		Issuer:    tokenClaims.Issuer,
		IssuedAt:  tokenClaims.IssuedAt.Time,
		ExpiresAt: tokenClaims.ExpiresAt.Time,
		Subject:   tokenClaims.Subject,
	}
	log.Printf("tokenStruct.Subject %s\n", tokenStruct.Subject)
	userID, _ := strconv.Atoi(tokenStruct.Subject)

	user, err := cfg.DB.FindUserByID(userID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "User not found")
		return
	}

	decoder := json.NewDecoder(r.Body)
	parameters := params{}
	err = decoder.Decode(&parameters)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't decode parameters")
		return
	}

	hashedPassword, err := HashPassword(parameters.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't hash the password")
	}

	updatedUser, err := cfg.DB.UpdateUser(user.ID, parameters.Email, hashedPassword)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create user")
		return
	}

	respondWithJSON(w, http.StatusOK, User{
		ID:    updatedUser.ID,
		Email: updatedUser.Email,
	})
}

func (cfg *apiConfig) handlerChirpsRetrieve(w http.ResponseWriter, r *http.Request) {
	dbChirps, err := cfg.DB.GetChirps()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve chirps")
		return
	}

	chirps := []Chirp{}
	for _, dbChirp := range dbChirps {
		chirps = append(chirps, Chirp{
			ID:   dbChirp.ID,
			Body: dbChirp.Body,
		})
	}

	sort.Slice(chirps, func(i, j int) bool {
		return chirps[i].ID < chirps[j].ID
	})

	respondWithJSON(w, http.StatusOK, chirps)

}

func (cfg *apiConfig) handleChirpRetrieval(w http.ResponseWriter, r *http.Request) {
	chirpId := r.PathValue("id")
	chirpID, err := strconv.Atoi(chirpId)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Mismatch of types")
		return
	}

	dbChirp, err := cfg.DB.GetChirp(chirpID)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "Couldn't get chirp")
		return
	}

	respondWithJSON(w, http.StatusOK, Chirp{
		ID:   dbChirp.ID,
		Body: dbChirp.Body,
	})
}

func getCleanedBody(body string, badWords map[string]struct{}) string {
	words := strings.Split(body, " ")
	for i, word := range words {
		loweredWord := strings.ToLower(word)
		if _, ok := badWords[loweredWord]; ok {
			words[i] = "****"
		}
	}
	cleaned := strings.Join(words, " ")
	return cleaned
}
