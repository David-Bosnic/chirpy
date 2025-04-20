package main

import (
	"fmt"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()
	mux.Handle("/app/", http.StripPrefix("/app/", http.FileServer(http.Dir("."))))

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(200)
		w.Write([]byte("OK"))
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
