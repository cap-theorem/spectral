package api

import "net/http"

func HandleHello(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("hello there"))
}
