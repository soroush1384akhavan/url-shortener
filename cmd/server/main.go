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
	"flag"
	"log"
	"net/http"

	_ "github.com/soroush1384akhavan/url-shortener/docs"
	"github.com/soroush1384akhavan/url-shortener/internal/httpapi"
	"github.com/soroush1384akhavan/url-shortener/internal/link"
	"github.com/soroush1384akhavan/url-shortener/internal/shortcode"
	"github.com/soroush1384akhavan/url-shortener/internal/store"
)

// @title URL Shortener API
// @version 1.0
// @description Simple URL shortener API
// @host localhost:8080
// @BasePath /
func main() {
	addr := flag.String("addr", ":8080", "server listen address")
	base := flag.String("base", "http://localhost:8080", "base URL for short links")

	flag.Parse()

	vldt := link.URLValidator{}
	st := store.NewMemoryStore()
	gn := shortcode.Base62Generator{}

	service := link.NewShortenerService(vldt, st, gn)

	handler := httpapi.NewHandler(service, *base)

	router := httpapi.NewRouter(handler)

	log.Println("server listening on :", *addr)

	if err := http.ListenAndServe(*addr, router); err != nil {
		log.Fatal(err)
	}
}
