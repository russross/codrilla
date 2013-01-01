package main

import (
	//"fmt"
	"github.com/gorilla/pat"
	"html/template"
	"log"
	"net/http"
)

var tmpl *template.Template

func main() {
	var err error
	if tmpl, err = template.ParseGlob("templates/*.template"); err != nil {
		log.Fatalf("Error loading templates: %v", err)
	}

	r := pat.New()
	r.Add("GET", `/admin/`, handlerAdminSession(admin_index))

	http.Handle("/css/", http.StripPrefix("/css", http.FileServer(http.Dir("css"))))
	http.Handle("/js/", http.StripPrefix("/js", http.FileServer(http.Dir("js"))))
	http.Handle("/img/", http.StripPrefix("/img", http.FileServer(http.Dir("img"))))
	http.Handle("/admin/", r)

	log.Fatal(http.ListenAndServe("127.0.0.1:8080", nil))
}

type handlerAdminSession func(http.ResponseWriter, *http.Request, *Session) error

func (h handlerAdminSession) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s := &Session{}
	err := h(w, r, s)
	if err != nil {
		log.Printf("Error: %v")
	}
}

type Session struct {
}

func admin_index(w http.ResponseWriter, r *http.Request, s *Session) error {
	return tmpl.ExecuteTemplate(w, "admin.template", nil)
}
