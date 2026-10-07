package main

// import (
// 	"fmt"

// 	"github.com/soroush1384akhavan/url-shortener/internal/link"
// 	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
// 	"github.com/soroush1384akhavan/url-shortener/internal/store"
// )

// func main(){
// 	validator := link.URLValidator{}
// 	st := store.NewMemoryStore()
// 	gn := shortcode.Base62Generator{}
// 	shortener := link.NewShortenerService(validator, st, gn)
// 	lnk, err := shortener.Shorten("https://example.com")
// 	if err != nil {
// 		fmt.Println(err)
// 	}

// 	lnk, exists := shortener.Store.FindByURL("hTtps://example.com")

// 	fmt.Println(lnk, exists)
// }

import (
	"net/http"

	"github.com/soroush1384akhavan/url-shortener/internal/httpapi"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/hello", httpapi.HelloHandler)

	http.ListenAndServe(":8080", mux)
}