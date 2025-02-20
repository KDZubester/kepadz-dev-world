package main

import (
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

var articleTemplate *template.Template
var blogTemplate *template.Template

// Article struct
type Article struct {
	ID      string
	Title   string
	Image   string
	Summary string
	Content template.HTML
}

// To hold articles
var articles []Article

func init() {
	var err error
	articleTemplate, err = template.ParseFiles("article.html")
	if err != nil {
		panic(err)
	}
	blogTemplate, err = template.ParseFiles("blog.html")
	if err != nil {
		panic(err)
	}
}

func main() {
	// Create a few articles with HTML content
	articles = append(articles, Article{
		ID:      "1",
		Title:   "Article 1",
		Image:   "/static/art/the_little_prince_favicon.jpg",
		Summary: "Summary of article 1",
		Content: template.HTML("<p>This is <strong>article 1</strong> content. It contains <a href='#'>links</a> and <em>formatted</em> text.</p>"),
	})

	r := mux.NewRouter()
	r.HandleFunc("/", serveIndex)
	r.HandleFunc("/blog", func(w http.ResponseWriter, r *http.Request) {
		// Pass articles to the blog page
		err := blogTemplate.ExecuteTemplate(w, "blog", articles)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	r.HandleFunc("/blog/article/{id}", articleHandler)        // Serve article dynamically
	r.HandleFunc("/blog/createArticle", serveCreateArticle)   // Serve the article creation form
	r.HandleFunc("/saveArticle", saveArticle).Methods("POST") // Save the new article
	r.HandleFunc("/{page}", servePage)

	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	http.Handle("/", r)
	log.Println("Server started on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func servePage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	page := vars["page"] + ".html"
	if _, err := os.Stat(page); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, page)
}

// Route to show the article creation form
func serveCreateArticle(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "createArticle.html")
}

// Handler for the articles
func articleHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

	// Find article by ID
	var article *Article
	for _, a := range articles {
		if a.ID == id {
			article = &a
			break
		}
	}

	if article == nil {
		http.NotFound(w, r)
		return
	}

	// Render article page
	err := articleTemplate.ExecuteTemplate(w, "article", article)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// Handle saving the new article from the form
func saveArticle(w http.ResponseWriter, r *http.Request) {
	// Get form data
	title := r.FormValue("title")
	image := r.FormValue("image")
	summary := r.FormValue("summary")
	content := r.FormValue("content")

	// Create a new article
	newArticle := Article{
		ID:      string(len(articles) + 1), // Create a new ID
		Title:   title,
		Image:   image,
		Summary: summary,
		Content: template.HTML(content),
	}

	// Save the article in the articles slice
	articles = append(articles, newArticle)

	// Redirect to the blog page
	http.Redirect(w, r, "/blog", http.StatusSeeOther)
}
