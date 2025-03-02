package main

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "database/sql"
    "encoding/base64"
    "fmt"
    "html/template"
    "io"
    "log"
    "net/http"
    "os"
    "sync"

    "github.com/gorilla/mux"
    "github.com/gorilla/sessions"
    "golang.org/x/crypto/bcrypt"
    _ "github.com/go-sql-driver/mysql"
)

var articleTemplate *template.Template
var blogTemplate *template.Template
var indexTemplate *template.Template
var db *sql.DB

// Article struct
type Article struct {
    ID      string
    Title   string
    Image   string
    Summary string
    Content template.HTML
}

type Permissions struct {
    IsAuthenticated bool
    IsAdmin         bool
}

type BlogPageData struct {
    Articles    []Article
    Permissions Permissions
}

type ArticlePageData struct {
    Article     Article
    Permissions Permissions
}

var store = sessions.NewCookieStore([]byte(os.Getenv("SESSION_KEY")))
var encryptionKey = []byte(os.Getenv("AES_KEY"))

type User struct {
    Username     string
    PasswordHash string
    Email        string
    ReceiveEmails bool
    Role         string
}

var users = make(map[string]User)
var mu sync.Mutex

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
    indexTemplate, err = template.ParseFiles("index.html")
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
    r.HandleFunc("/signup", serveSignup).Methods("GET", "POST")
    r.HandleFunc("/login", serveLogin).Methods("GET", "POST")
    r.HandleFunc("/logout", serveLogout)

    r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

    http.Handle("/", r)
    log.Println("Server started on http://localhost:8080")
    if err := http.ListenAndServe(":8080", nil); err != nil {
        log.Fatalf("Server failed to start: %v", err)
    }
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
    permissions := Permissions{
        IsAuthenticated: isAuthenticated(r),
        IsAdmin:         hasRole(r, "admin"),
    }

    err := indexTemplate.ExecuteTemplate(w, "index", permissions)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
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

    data := BlogPageData{
        Articles: articles,
        Permissions: Permissions{
            IsAuthenticated: isAuthenticated(r),
            IsAdmin:         hasRole(r, "admin"),
        },
    }
	log.Printf("Permissions: IsAuthenticated=%v, IsAdmin=%v", data.Permissions.IsAuthenticated, data.Permissions.IsAdmin)

    err = blogTemplate.ExecuteTemplate(w, "blog", data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func serveCreateArticle(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "createArticle.html")
}

func articleHandler(w http.ResponseWriter, r *http.Request) {
    id := mux.Vars(r)["id"]

    var article Article
    err := db.QueryRow("SELECT id, title, image, summary, content FROM articles WHERE id = ?", id).Scan(&article.ID, &article.Title, &article.Image, &article.Summary, &article.Content)
    if err == sql.ErrNoRows {
        http.NotFound(w, r)
        return
    } else if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    data := ArticlePageData{
        Article: article,
        Permissions: Permissions{
            IsAuthenticated: isAuthenticated(r),
            IsAdmin:         hasRole(r, "admin"),
        },
    }

    err = articleTemplate.ExecuteTemplate(w, "article", data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func saveArticle(w http.ResponseWriter, r *http.Request) {
    title := r.FormValue("title")
    image := r.FormValue("image")
    summary := r.FormValue("summary")
    content := r.FormValue("content")

    _, err := db.Exec("INSERT INTO articles (title, image, summary, content) VALUES (?, ?, ?, ?)", title, image, summary, content)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    http.Redirect(w, r, "/blog", http.StatusSeeOther)
}

func hashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
    return string(bytes), err
}

func checkPasswordHash(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}

func encryptEmail(email string) (string, error) {
    block, err := aes.NewCipher(encryptionKey)
    if err != nil {
        return "", err
    }

    b := base64.StdEncoding.EncodeToString([]byte(email))
    ciphertext := make([]byte, aes.BlockSize+len(b))
    iv := ciphertext[:aes.BlockSize]
    if _, err := io.ReadFull(rand.Reader, iv); err != nil {
        return "", err
    }

    stream := cipher.NewCFBEncrypter(block, iv)
    stream.XORKeyStream(ciphertext[aes.BlockSize:], []byte(b))

    return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func decryptEmail(encryptedEmail string) (string, error) {
    ciphertext, _ := base64.URLEncoding.DecodeString(encryptedEmail)

    block, err := aes.NewCipher(encryptionKey)
    if err != nil {
        return "", err
    }

    if len(ciphertext) < aes.BlockSize {
        return "", fmt.Errorf("ciphertext too short")
    }
    iv := ciphertext[:aes.BlockSize]
    ciphertext = ciphertext[aes.BlockSize:]

    stream := cipher.NewCFBDecrypter(block, iv)
    stream.XORKeyStream(ciphertext, ciphertext)

    data, err := base64.StdEncoding.DecodeString(string(ciphertext))
    if err != nil {
        return "", err
    }

    return string(data), nil
}

func serveSignup(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        http.ServeFile(w, r, "signup.html")
        return
    }

    username := r.FormValue("username")
    password := r.FormValue("password")
    email := r.FormValue("email")
    receiveEmails := r.FormValue("receive_emails") == "on"

    passwordHash, err := hashPassword(password)
    if err != nil {
        log.Printf("Error hashing password: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    encryptedEmail, err := encryptEmail(email)
    if err != nil {
        log.Printf("Error encrypting email: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    _, err = db.Exec("INSERT INTO users (username, password_hash, email, receive_emails, role) VALUES (?, ?, ?, ?, ?)",
        username, passwordHash, encryptedEmail, receiveEmails, "user")
    if err != nil {
        log.Printf("Error inserting user into database: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    log.Printf("User %s signed up successfully", username)
    http.Redirect(w, r, "/", http.StatusSeeOther)
}

func serveLogin(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        http.ServeFile(w, r, "login.html")
        return
    }

    username := r.FormValue("username")
    password := r.FormValue("password")

    var passwordHash, encryptedEmail, role string
    var receiveEmails bool
    err := db.QueryRow("SELECT password_hash, email, receive_emails, role FROM users WHERE username = ?", username).Scan(&passwordHash, &encryptedEmail, &receiveEmails, &role)
    if err != nil {
        if err == sql.ErrNoRows {
            log.Printf("Invalid username or password for user: %s", username)
            http.Error(w, "Invalid username or password", http.StatusUnauthorized)
        } else {
            log.Printf("Error querying user from database: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
        }
        return
    }

    if !checkPasswordHash(password, passwordHash) {
        log.Printf("Invalid password for user: %s", username)
        http.Error(w, "Invalid username or password", http.StatusUnauthorized)
        return
    }

    session, _ := store.Get(r, "session")
    session.Values["user_id"] = username
    session.Values["role"] = role
    err = session.Save(r, w)
    if err != nil {
        log.Printf("Error saving session: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    log.Printf("User %s logged in successfully", username)
    log.Printf("Session values: user_id=%v, role=%v", session.Values["user_id"], session.Values["role"])
    http.Redirect(w, r, "/", http.StatusSeeOther)
}

func serveLogout(w http.ResponseWriter, r *http.Request) {
    session, _ := store.Get(r, "session")
    delete(session.Values, "user_id")
    delete(session.Values, "role")
    session.Save(r, w)

    http.Redirect(w, r, "/", http.StatusSeeOther)
}

func isAuthenticated(r *http.Request) bool {
    session, _ := store.Get(r, "session")
    userID, ok := session.Values["user_id"]
    log.Printf("isAuthenticated: user_id=%v, ok=%v", userID, ok)
    return ok
}

func hasRole(r *http.Request, role string) bool {
    session, _ := store.Get(r, "session")
    userRole, ok := session.Values["role"]
    log.Printf("hasRole: user_role=%v, ok=%v", userRole, ok)
    if !ok {
        return false
    }

    return userRole == role
}