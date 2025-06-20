package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"sync/atomic"
	"time"

	"github.com/David-Bosnic/chirpy/internal/auth"
	"github.com/David-Bosnic/chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileserverHit atomic.Int32
	queries       *database.Queries
	platform      string
	JWTSecret     string
}

type User struct {
	ID          uuid.UUID `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Email       string    `json:"email"`
	IsChirpyRed bool      `json:"is_chirpy_red"`
}

type UserWithJWT struct {
	User
	Token        string `json:"token"`
	RefreshToken string `json:"refresh_token"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("Error connection to db:", err)
	}
	dbQueries := database.New(db)
	var apiConf apiConfig
	apiConf.queries = dbQueries
	apiConf.platform = os.Getenv("PLATFORM")
	apiConf.JWTSecret = os.Getenv("SECRET")
	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app/", apiConf.middlewareMetricsInc(http.FileServer(http.Dir(".")))))

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("GET /admin/metrics", func(w http.ResponseWriter, r *http.Request) {
		body := fmt.Sprintf(`
<html>
  <body>
    <h1>Welcome, Chirpy Admin</h1>
    <p>Chirpy has been visited %d times!</p>
  </body>
</html>
			`, apiConf.fileserverHit.Load())
		w.Header().Add("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte(body))

	})
	mux.HandleFunc("POST /admin/reset", func(w http.ResponseWriter, r *http.Request) {
		apiConf.fileserverHit.Store(0)
		err := dbQueries.DeleteAllUsers(r.Context())
		if err != nil {
			log.Printf("Error deleting all users: %s", err)
			return
		}
		log.Print("Reset DB and Server Complete")
		w.WriteHeader(200)
	})
	mux.HandleFunc("GET /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		s := r.URL.Query().Get("author_id")
		var parsedID uuid.UUID
		if s != "" {
			parsedID, err = uuid.Parse(s)
			if err != nil {
				log.Printf("Error parsing GET chirps id: %s", err)
				return
			}
		}
		var untaggedChirps []database.Chirp
		if s != "" {
			untaggedChirps, err = dbQueries.ListChirpsFromAuthorID(r.Context(), parsedID)
		} else {
			untaggedChirps, err = dbQueries.ListChirps(r.Context())
		}
		if err != nil {
			log.Printf("Error getting all chirps: %s", err)
			return
		}
		taggedChirps := []Chirp{}
		for _, untaggedChirp := range untaggedChirps {
			taggedChirps = append(taggedChirps, addTagsToChirp(untaggedChirp))
		}

		sortType := r.URL.Query().Get("sort")
		if sortType == "asc" {
			sort.Slice(taggedChirps, func(i, j int) bool { return taggedChirps[i].CreatedAt.Before(taggedChirps[j].CreatedAt) })
		} else if sortType == "desc" {
			sort.Slice(taggedChirps, func(i, j int) bool { return taggedChirps[i].CreatedAt.After(taggedChirps[j].CreatedAt) })
		}
		dat, err := json.Marshal(taggedChirps)
		if err != nil {
			log.Printf("Error marshaling list of chirps: %s", err)
			return
		}
		w.WriteHeader(200)
		w.Write(dat)

	})
	mux.HandleFunc("GET /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("chirpID")
		parsedID, err := uuid.Parse(id)
		if err != nil {
			log.Printf("Error parsing GET chirps id: %s", err)
			return
		}
		chirp, err := dbQueries.GetChirp(r.Context(), parsedID)
		if err != nil {
			log.Printf("Error getting single chirp: %s", err)
			w.WriteHeader(404)
			w.Write([]byte("Failed to get chirp"))
			return
		}

		dat, err := json.Marshal(addTagsToChirp(chirp))
		if err != nil {
			log.Printf("Error marsheling single chirp: %s", err)
			w.WriteHeader(500)
			w.Write([]byte("Failed to marshel chirp"))
			return
		}

		w.WriteHeader(200)
		w.Write(dat)

	})
	mux.HandleFunc("POST /api/chirps", func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Body string `json:"body"`
		}
		token, err := auth.GetBearerToken(r.Header)
		if err != nil {
			log.Printf("Error failed to get bearer token %s\n", err)
			w.WriteHeader(400)
			w.Write([]byte("Failed to get bearer token"))
			return
		}

		validatedUUID, err := auth.ValidateJWT(token, apiConf.JWTSecret)
		if err != nil {
			log.Printf("Error failed to validate jwt %s\n", err)
			w.WriteHeader(401)
			w.Write([]byte("Failed to validate jwt token"))
			return
		}
		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err = decoder.Decode(&params)
		if err != nil {
			resp := struct {
				Error string `json:"error"`
			}{
				Error: "Something went wrong",
			}
			log.Printf("Error decoding chirps JSON : %s", err)
			dat, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Error marshaling chirps JSON: %s", err)
				w.Write(dat)
				return
			}
		}
		if len(params.Body) > 140 {
			resp := struct {
				Error string `json:"error"`
			}{
				Error: "Chirp is too long",
			}
			dat, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Error marshaling JSON: %s", err)
				return
			}
			w.WriteHeader(400)
			w.Write(dat)
			return
		}
		if len(params.Body) == 0 {
			resp := struct {
				Error string `json:"error"`
			}{
				Error: "Chirp has nothing in the body",
			}
			dat, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Error marshaling JSON: %s", err)
				return
			}
			w.WriteHeader(400)
			w.Write(dat)
			return
		}
		// cleanUUID, err := uuid.Parse(validatedUUID)
		// if err != nil {
		// 	log.Printf("Error parsing uuid for chirps: %s", err)
		// 	return
		// }

		cleanChirp := database.CreateChirpParams{
			Body:   cleanBody(params.Body),
			UserID: validatedUUID,
		}
		chirp, err := dbQueries.CreateChirp(r.Context(), cleanChirp)
		if err != nil {
			log.Printf("Error creating chirp: %s", err)
		}
		formattedChirp := Chirp{
			ID:        chirp.ID,
			CreatedAt: chirp.CreatedAt,
			UpdatedAt: chirp.UpdatedAt,
			Body:      chirp.Body,
			UserID:    chirp.UserID,
		}

		dat, err := json.Marshal(formattedChirp)
		if err != nil {
			log.Printf("Error marshaling JSON: %s", err)
			return
		}
		w.WriteHeader(201)
		w.Write(dat)
		return

	})
	mux.HandleFunc("POST /api/users", func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			resp := struct {
				Error string `json:"error"`
			}{
				Error: "Something went wrong",
			}
			log.Printf("Error decoding users JSON: %s", err)
			dat, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Error marshaling users JSON: %s", err)
				w.Write(dat)
				return
			}
		}
		hashedPass, err := auth.HashPassword(params.Password)
		if err != nil {
			log.Printf("Error hashing new user password: %s", err)
			return
		}
		formatedParams := database.CreateUserParams{
			Email:          params.Email,
			HashedPassword: hashedPass,
		}

		user, err := dbQueries.CreateUser(r.Context(), formatedParams)
		if err != nil {
			log.Printf("Error Creating User: %s", err)
			w.WriteHeader(500)
			return
		}
		formatedUser := User{
			ID:          user.ID,
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.UpdatedAt,
			Email:       user.Email,
			IsChirpyRed: user.IsChirpyRed,
		}
		dat, err := json.Marshal(formatedUser)
		if err != nil {
			log.Printf("Error marshaling JSON: %s", err)
			w.Write(dat)
			return
		}
		w.WriteHeader(201)
		w.Write(dat)
	})
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		if err != nil {
			resp := struct {
				Error string `json:"error"`
			}{
				Error: "Something went wrong",
			}
			log.Printf("Error decoding users JSON: %s", err)
			dat, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Error marshaling users JSON: %s", err)
				w.Write(dat)
				return
			}
		}
		user, err := dbQueries.GetUserByEmail(r.Context(), params.Email)
		if err != nil {
			log.Printf("Error getting user by email: %s", err)
			return
		}
		err = auth.CheckPasswordHash(user.HashedPassword, params.Password)
		if err != nil {
			w.WriteHeader(401)
			w.Write([]byte("incorrect email or password"))
			return
		}
		jwtToken, err := auth.MakeJWT(user.ID, apiConf.JWTSecret)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("Failed to make JWT token"))
			return
		}

		refreshToken, _ := auth.MakeRefreshTokenString()
		tokenparams := database.CreateRefreshTokenParams{
			Token:     refreshToken,
			ExpiresAt: time.Now().AddDate(0, 0, 60),
			RevokedAt: sql.NullTime{
				Valid: false,
			},
			UserID: user.ID,
		}

		dbQueries.CreateRefreshToken(r.Context(), tokenparams)
		formatedUser := UserWithJWT{
			User: User{
				ID:          user.ID,
				CreatedAt:   user.CreatedAt,
				UpdatedAt:   user.UpdatedAt,
				Email:       user.Email,
				IsChirpyRed: user.IsChirpyRed,
			},
			Token:        jwtToken,
			RefreshToken: refreshToken,
		}

		dat, err := json.Marshal(formatedUser)
		if err != nil {
			log.Printf("POST /api/login/ Error marshaling user: %s", err)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(dat))
	})
	mux.HandleFunc("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) {
		bearerToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			log.Printf("Error failed to get bearer token %s\n", err)
			w.WriteHeader(400)
			w.Write([]byte("Failed to get bearer token"))
			return
		}

		refreshToken, err := dbQueries.GetRefreshToken(r.Context(), bearerToken)
		if err != nil || time.Now().After(refreshToken.ExpiresAt) || refreshToken.RevokedAt.Valid {
			w.WriteHeader(401)
			w.Write([]byte("Failed to find refresh token or expired token"))
			return
		}
		user, err := dbQueries.GetUserFromRefreshToken(r.Context(), bearerToken)
		if err != nil {
			w.WriteHeader(401)
			w.Write([]byte("Failed to find user from refresh token"))
			return
		}
		jwtToken, err := auth.MakeJWT(user.ID, apiConf.JWTSecret)
		if err != nil {
			w.WriteHeader(500)
			w.Write([]byte("Failed to make JWT token"))
			return
		}

		resp := struct {
			Token string `json:"token"`
		}{
			Token: jwtToken,
		}
		dat, err := json.Marshal(resp)
		if err != nil {
			log.Printf("Error marshaling JSON: %s", err)
			w.Write(dat)
			return
		}
		w.WriteHeader(200)
		w.Write(dat)
	})
	mux.HandleFunc("POST /api/revoke", func(w http.ResponseWriter, r *http.Request) {
		bearerToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			log.Printf("Error failed to get bearer token %s\n", err)
			w.WriteHeader(400)
			w.Write([]byte("Failed to get bearer token"))
			return
		}
		dbQueries.RevokeRefreshToken(r.Context(), bearerToken)
		w.WriteHeader(204)
		return

	})
	mux.HandleFunc("PUT /api/users", func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}

		bearerToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			log.Printf("Error failed to get bearer token %s\n", err)
			w.WriteHeader(401)
			w.Write([]byte("Failed to get bearer token"))
			return
		}
		userId, err := auth.ValidateJWT(bearerToken, apiConf.JWTSecret)
		if err != nil {
			log.Printf("Error Failed to validate JWT %s\n", err)
			w.WriteHeader(401)
			w.Write([]byte("Failed to validate JWT"))
			return
		}
		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err = decoder.Decode(&params)
		if err != nil {
			resp := struct {
				Error string `json:"error"`
			}{
				Error: "Something went wrong",
			}
			log.Printf("Error decoding users JSON: %s", err)
			dat, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Error marshaling users JSON: %s", err)
				w.Write(dat)
				return
			}
		}

		hashedPass, err := auth.HashPassword(params.Password)
		if err != nil {
			w.WriteHeader(500)
			log.Printf("Error hashing password: %s", err)
			return
		}
		formatedRequest := database.UpdateUserEmailAndPasswordParams{
			Email:          params.Email,
			HashedPassword: hashedPass,
			ID:             userId,
		}
		err = dbQueries.UpdateUserEmailAndPassword(r.Context(), formatedRequest)
		if err != nil {
			w.WriteHeader(500)
			log.Printf("Error updating user email and password: %s", err)
			return
		}
		user, err := dbQueries.GetUserByEmail(r.Context(), params.Email)
		if err != nil {
			w.WriteHeader(500)
			log.Printf("Error getting user: %s", err)
			return
		}
		resp := User{
			ID:          user.ID,
			CreatedAt:   user.CreatedAt,
			UpdatedAt:   user.UpdatedAt,
			Email:       user.Email,
			IsChirpyRed: user.IsChirpyRed,
		}

		dat, err := json.Marshal(resp)
		if err != nil {
			log.Printf("Error marshaling JSON: %s", err)
			return
		}
		w.WriteHeader(200)
		w.Write(dat)
		return
	})
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("chirpID")
		parsedID, err := uuid.Parse(id)
		if err != nil {
			log.Printf("Error parsing GET chirps id: %s", err)
			return
		}
		bearerToken, err := auth.GetBearerToken(r.Header)
		if err != nil {
			log.Printf("Error failed to get bearer token %s\n", err)
			w.WriteHeader(401)
			w.Write([]byte("Failed to get bearer token"))
			return
		}
		userId, err := auth.ValidateJWT(bearerToken, apiConf.JWTSecret)
		if err != nil {
			log.Printf("Error Failed to validate JWT %s\n", err)
			w.WriteHeader(403)
			w.Write([]byte("Failed to validate JWT"))
			return
		}
		currentChirp, err := dbQueries.GetChirp(r.Context(), parsedID)
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("Chirp does not exist"))
			return
		}
		if currentChirp.UserID != userId {
			w.WriteHeader(403)
			w.Write([]byte("Error User id and chirp owner do not match"))
			return
		}
		err = dbQueries.DeleteChirp(r.Context(), currentChirp.ID)
		if err != nil {
			log.Printf("Failed to delete chirp")
			w.WriteHeader(404)
			w.Write([]byte("Chirp was not found"))
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /api/polka/webhooks", func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Event string `json:"event"`
			Data  struct {
				UserID string `json:"user_id"`
			} `json:"data"`
		}

		decoder := json.NewDecoder(r.Body)
		params := parameters{}
		err := decoder.Decode(&params)
		apikey, err := auth.GetApiKey(r.Header)
		if err != nil {
			log.Printf("Error failed to get Api Key token %s\n", err)
			w.WriteHeader(401)
			w.Write([]byte("Failed to get Api Key"))
			return
		}
		polkaKey := os.Getenv("POLKA_KEY")
		if apikey != polkaKey {
			log.Printf("Error Api key did not match: %s\n", err)
			w.WriteHeader(401)
			w.Write([]byte("Api key did not match"))
			return
		}
		if err != nil {
			resp := struct {
				Error string `json:"error"`
			}{
				Error: "Something went wrong",
			}
			log.Printf("Error decoding users JSON: %s", err)
			dat, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Error marshaling users JSON: %s", err)
				w.Write(dat)
				return
			}
		}
		if params.Event != "user.upgraded" {
			log.Printf("Error event was not 'user.upgraded'")
			w.WriteHeader(204)
			return
		}
		parsedID, err := uuid.Parse(params.Data.UserID)
		if err != nil {
			w.WriteHeader(400)
			w.Write([]byte("Error parsing userID"))
			return
		}
		err = dbQueries.UpdateToChirpyRed(r.Context(), parsedID)
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("User not found to Update to Chirpy Red"))
			return
		}
		w.WriteHeader(204)
		return

	})
	ServerMux := http.Server{}
	ServerMux.Handler = mux
	ServerMux.Addr = ":8080"

	fmt.Println("Running Server")
	err = ServerMux.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	} else {
		fmt.Println("Spinning up server")
	}
}

func cleanBody(txt string) string {
	re := regexp.MustCompile("(?i)(kerfuffle|sharbert|fornax)")
	cleanTxt := re.ReplaceAllString(txt, "****")
	return cleanTxt
}

func addTagsToChirp(noTagChirp database.Chirp) Chirp {
	return Chirp{
		ID:        noTagChirp.ID,
		CreatedAt: noTagChirp.CreatedAt,
		UpdatedAt: noTagChirp.UpdatedAt,
		Body:      noTagChirp.Body,
		UserID:    noTagChirp.UserID,
	}
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHit.Add(1)
		next.ServeHTTP(w, r)
	})
}
