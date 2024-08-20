package main

import (
	"fmt"
	"net/http"

	"github.com/BGrewell/go-iperf"
)

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "Hello, World!")
}

func main() {
	s := iperf.NewServer()
	s.SetPort(5201)
	err := s.Start()
	if err != nil {
		fmt.Println("Failed to start server:", err)
		return
	}
	defer s.Stop()
	fmt.Println("Server is running...")

	http.HandleFunc("/", helloHandler)
	if err := http.ListenAndServe(":8080", nil); err != nil {
		panic(err)
	}
}
