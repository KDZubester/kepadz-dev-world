package main

import (
	"html/template"
	"log"
	"net/http"
	"os"
    "fmt"
    "database/sql"

	"github.com/gorilla/mux"
	_ "github.com/go-sql-driver/mysql"
)

var articleTemplate *template.Template
var blogTemplate *template.Template
var db *sql.DB

// Article struct
type Article struct {
	ID      string
	Title   string
	Image   string
	Summary string
	Content template.HTML
}

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

    // Initialize database connection using environment variables
    dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s",
        os.Getenv("DB_USER"),
        os.Getenv("DB_PASSWORD"),
        os.Getenv("DB_HOST"),
        os.Getenv("DB_PORT"),
        os.Getenv("DB_NAME"))
    db, err = sql.Open("mysql", dsn)
    if err != nil {
        panic(err)
    }

    // Test the database connection
    err = db.Ping()
    if err != nil {
        panic(err)
    }
}

func main() {

	r := mux.NewRouter()
	r.HandleFunc("/", serveIndex)
	r.HandleFunc("/blog", serveBlog)
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

func serveBlog(w http.ResponseWriter, r *http.Request) {
    rows, err := db.Query("SELECT id, title, image, summary, content FROM articles")
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    defer rows.Close()

    var articles []Article
    for rows.Next() {
        var article Article
        err := rows.Scan(&article.ID, &article.Title, &article.Image, &article.Summary, &article.Content)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        articles = append(articles, article)
    }

    err = blogTemplate.ExecuteTemplate(w, "blog", articles)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

// Route to show the article creation form
func serveCreateArticle(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "createArticle.html")
}

// Handler for the articles
func articleHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]

    // Find article by ID
    var article Article
    err := db.QueryRow("SELECT id, title, image, summary, content FROM articles WHERE id = ?", id).Scan(&article.ID, &article.Title, &article.Image, &article.Summary, &article.Content)
    if err == sql.ErrNoRows {
        http.NotFound(w, r)
        return
    } else if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	// Render article page
	err = articleTemplate.ExecuteTemplate(w, "article", article)
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

	// Insert the new article into the database
    _, err := db.Exec("INSERT INTO articles (title, image, summary, content) VALUES (?, ?, ?, ?)", title, image, summary, content)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

	// Redirect to the blog page
	http.Redirect(w, r, "/blog", http.StatusSeeOther)
}