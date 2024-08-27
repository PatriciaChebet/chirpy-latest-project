package main

import ("log"
"net/http"
"fmt"
"encoding/json"
"strings"
"github.com/PatriciaChebet/chirpy-latest-project/database"
)

type apiConfig struct {
	fileserveHits int
	DB *database.DB
}

type Chirp struct {
	ID int `json:"id"`
	Body string `json:"body"`
}

func main(){
	const filepathRoot = "."
	const port = "8080"

	db, err := database.NewDB("database.json")
	if err != nil {
		log.Fatal(err)
	}
	
	apiCfg := apiConfig{
		fileserveHits: 0,
		DB: db,
	}

	mux := http.NewServeMux()
	mux.Handle("/app/*", apiCfg.middlewareMetricsInc(http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))))
	mux.Handle("/admin/*", apiCfg.middlewareMetricsInc(http.StripPrefix("/admin", http.FileServer(http.Dir(filepathRoot)))))
	mux.HandleFunc("GET /api/healthz", calcHealthz)
	mux.HandleFunc("GET /admin/metrics", apiCfg.calcServerHits)
	mux.HandleFunc("GET /api/reset", apiCfg.resetServerHits)
	mux.HandleFunc("POST /api/chirps", apiCfg.handlerChirpsCreate)
	mux.HandleFunc("GET /api/chirps", apiCfg.handlerChirpsRetrieve)

	srv := &http.Server{
		Addr: ":" + port,
		Handler: mux,
	}

	log.Printf("Server started on port %s\n", port)
	log.Fatal(srv.ListenAndServe())
}

func calcHealthz(w http.ResponseWriter, r *http.Request){
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (cfg *apiConfig) calcServerHits(w http.ResponseWriter, r *http.Request){
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("<h1>Welcome, Chirpy Admin</h1>"))
	w.Write([]byte(fmt.Sprintf("Chirpy has been visited %d times!", cfg.fileserveHits)))
}

func (cfg *apiConfig) resetServerHits(w http.ResponseWriter, r *http.Request){
	cfg.fileserveHits = 0
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset"))
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request){
		cfg.fileserveHits++
		next.ServeHTTP(w, r)
	})
}

func validate_chirp(w http.ResponseWriter, r *http.Request){
	type chirp struct {
		Body string `json:"body"`
	}

	type validatedChirp struct {
		Id int `json:"id"`
		CleanedBody string `json:"cleaned_body"`
	}

	decoder := json.NewDecoder(r.Body)
	params := chirp{}
	err := decoder.Decode(&params)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't decode parameters")
		return
	}

	const maxChirpLength = 140
	
	if len(params.Body) > maxChirpLength {
		respondWithError(w, http.StatusBadRequest, "Chirp is too long")
		return
	}

	var replacedParam = strings.NewReplacer("kerfuffle", "****",
	"Kerfuffle", "****", "sharbert", "****", "Sharbert", "****", "fornax", "****", "Fornax", "****").Replace(params.Body)

	respondWithJSON(w, http.StatusOK, validatedChirp{
		CleanedBody: replacedParam,
	})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}){
	response, err := json.Marshal(payload)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't marshal response")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func respondWithError(w http.ResponseWriter, code int, message string){
	if code > 499 {
		log.Printf("Responding with error: %s", message)
	}

	type errorResponse struct {	
		Error string `json:"error"`
	}

	respondWithJSON(w, code, errorResponse{Error: message})
}

func(cfg *apiConfig) handleChirpsCreate(w http.ResponseWriter, r *http.Request){
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

	cleaned, err := validatedChirp(params.Body)
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

func(cfg *apiConfig) handlerChirpsRetrieve(w http.ResponseWriter, r *http.Request) {
	dbChirps, err := cfg.DB.GetChirps()
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't retrieve chirps")
		return
	}

	chirps := []Chirp{}
	for _, dbChirp := range dbChirps{
		chirps = append(chrips, Chirp{
			ID: dbChirp.ID,
			Body: dbChirp.Body,
		})
	}

	sort.Slice(chirps, func(i, j int) bool{
		return chirps[i].ID < chirps[j].ID
	})

	respondWithJSON(w, http.StatusOK, chirps)

}

