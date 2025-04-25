package main

import _ "github.com/lib/pq"
import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sync/atomic"
)

type apiConfig struct {
	fileserverHit atomic.Int32
}

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileserverHit.Add(1)
		next.ServeHTTP(w, r)
	})
}

func main() {
	var apiConf apiConfig
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
		w.WriteHeader(200)
	})
	mux.HandleFunc("POST /api/validate_chirp", func(w http.ResponseWriter, r *http.Request) {
		type parameters struct {
			Body string `json:"body"`
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
			log.Printf("Error decoding JSON: %s", err)
			dat, err := json.Marshal(resp)
			if err != nil {
				log.Printf("Error marshaling JSON: %s", err)
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

		cleanedBody := cleanBody(params.Body)
		resp := struct {
			CleanedBody string `json:"cleaned_body"`
		}{
			CleanedBody: cleanedBody,
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
	ServerMux := http.Server{}
	ServerMux.Handler = mux
	ServerMux.Addr = ":8080"

	fmt.Println("Running Server")
	err := ServerMux.ListenAndServe()
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
