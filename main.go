package main

import (
	"crypto/tls"
    "database/sql"
    "fmt"
    "html/template"
    "log"
    "net/http"
    "os"
    "sync"
	"net/smtp"

    "github.com/gorilla/mux"
    "github.com/gorilla/sessions"
    "golang.org/x/crypto/bcrypt"
    _ "github.com/go-sql-driver/mysql"
)

var articleTemplate *template.Template
var blogTemplate *template.Template
var indexTemplate *template.Template
var accountTemplate *template.Template
var contactTemplate *template.Template
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

type AccountPageData struct {
    User        User
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
    accountTemplate, err = template.ParseFiles("account.html")
    if err != nil {
        panic(err)
    }
    contactTemplate, err = template.ParseFiles("contact.html")
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
    r.HandleFunc("/account", serveAccount).Methods("GET")
    r.HandleFunc("/deleteAccount", serveDeleteAccount).Methods("POST")
	r.HandleFunc("/unsubscribe", serveUnsubscribe).Methods("GET", "POST")
    r.HandleFunc("/forgotUsername", serveForgotUsername).Methods("GET", "POST")
    r.HandleFunc("/forgotPassword", serveForgotPassword).Methods("GET", "POST")
    r.HandleFunc("/resetPassword", serveResetPassword).Methods("GET", "POST")
    r.HandleFunc("/contact", serveContact).Methods("GET", "POST")

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

    // Send email to users who opted to receive emails
    go sendEmailToSubscribers(title, summary)

    http.Redirect(w, r, "/blog", http.StatusSeeOther)
}

func sendEmailToSubscribers(title, summary string) {
    // Retrieve users who opted to receive emails
    rows, err := db.Query("SELECT email FROM users WHERE receive_emails = TRUE")
    if err != nil {
        log.Printf("Error querying users: %v", err)
        return
    }
    defer rows.Close()

    var emails []string
    for rows.Next() {
        var email string
        err := rows.Scan(&email)
        if err != nil {
            log.Printf("Error scanning email: %v", err)
            continue
        }

        log.Printf("Email to be sent to: %s", email)
        emails = append(emails, email)
    }

    // SMTP server configuration
    smtpHost := os.Getenv("SMTP_HOST")
    smtpPort := os.Getenv("SMTP_PORT")
    smtpUser := os.Getenv("SMTP_USER")
    smtpPass := os.Getenv("SMTP_PASS")

    auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

    from := smtpUser
    subject := "New Article Published: " + title
    body := fmt.Sprintf(`
        <html>
        <body>
        <p>A new article has been published on our blog:</p>
        <p><strong>%s</strong></p>
        <p>%s</p>
        <p>Visit our blog to read the full article.</p>
        <p>If you no longer wish to receive these emails, you can <a href="http://localhost:8080/unsubscribe?email=%%s">unsubscribe</a>.</p>
        </body>
        </html>
    `, title, summary)

    for _, to := range emails {
        msg := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\nMIME-Version: 1.0\nContent-Type: text/html; charset=\"UTF-8\"\nContent-Transfer-Encoding: 7bit\n\n%s", from, to, subject, fmt.Sprintf(body, to))

        // Connect to the SMTP server
        conn, err := smtp.Dial(smtpHost + ":" + smtpPort)
        if err != nil {
            log.Printf("Error connecting to SMTP server: %v", err)
            continue
        }

        // Start TLS
        tlsconfig := &tls.Config{
            InsecureSkipVerify: true,
            ServerName:         smtpHost,
        }

        if err = conn.StartTLS(tlsconfig); err != nil {
            log.Printf("Error starting TLS: %v", err)
            continue
        }

        // Authenticate
        if err = conn.Auth(auth); err != nil {
            log.Printf("Error authenticating to SMTP server: %v", err)
            continue
        }

        // Set the sender and recipient
        if err = conn.Mail(from); err != nil {
            log.Printf("Error setting sender: %v", err)
            continue
        }
        if err = conn.Rcpt(to); err != nil {
            log.Printf("Error setting recipient: %v", err)
            continue
        }

        // Send the email body
        w, err := conn.Data()
        if err != nil {
            log.Printf("Error getting Data writer: %v", err)
            continue
        }
        _, err = w.Write([]byte(msg))
        if err != nil {
            log.Printf("Error writing email body: %v", err)
            continue
        }
        err = w.Close()
        if err != nil {
            log.Printf("Error closing Data writer: %v", err)
            continue
        }

        // Close the connection
        conn.Quit()

        log.Printf("Email sent to %s", to)
    }
}
func hashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
    return string(bytes), err
}

func checkPasswordHash(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
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

    _, err = db.Exec("INSERT INTO users (username, password_hash, email, receive_emails, role) VALUES (?, ?, ?, ?, ?)",
        username, passwordHash, email, receiveEmails, "user")
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

func serveAccount(w http.ResponseWriter, r *http.Request) {
    session, _ := store.Get(r, "session")
    username, ok := session.Values["user_id"].(string)
    if !ok || username == "" {
        http.Redirect(w, r, "/login", http.StatusSeeOther)
        return
    }

    // Query the database for the user's information
    var user User
    err := db.QueryRow("SELECT username, email, receive_emails, role FROM users WHERE username = ?", username).
        Scan(&user.Username, &user.Email, &user.ReceiveEmails, &user.Role)
    if err != nil {
        log.Printf("Error retrieving user information: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    permissions := Permissions{
        IsAuthenticated: isAuthenticated(r),
        IsAdmin:         hasRole(r, "admin"),
    }

    accountData := AccountPageData{
        User: user,
        Permissions: permissions,
    }

    err = accountTemplate.ExecuteTemplate(w, "account", accountData)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}

func serveUnsubscribe(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        email := r.URL.Query().Get("email")

        data := struct {
            Email string
        }{
            Email: email,
        }
        tmpl, err := template.ParseFiles("unsubscribe.html")
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
            return
        }
        tmpl.Execute(w, data)
        return
    }

    if r.Method == http.MethodPost {
        email := r.FormValue("email")

        // If no email is provided in the URL, check the session for the logged-in user's email
        if email == "" {
            session, _ := store.Get(r, "session")
            username, ok := session.Values["user_id"].(string)
            if !ok || username == "" {
                http.Redirect(w, r, "/login", http.StatusSeeOther)
                return
            }

            // Query the database for the user's email
            var dbEmail string
            err := db.QueryRow("SELECT email FROM users WHERE username = ?", username).Scan(&dbEmail)
            if err != nil {
                log.Printf("Error retrieving email for user %s: %v", username, err)
                http.Error(w, "Internal server error", http.StatusInternalServerError)
                return
            }
            email = dbEmail
        }

        result, err := db.Exec("UPDATE users SET receive_emails = FALSE WHERE email = ?", email)
        if err != nil {
            log.Printf("Error updating user in database: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        rowsAffected, err := result.RowsAffected()
        if err != nil {
            log.Printf("Error getting rows affected: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        log.Printf("User with email %s unsubscribed successfully, rows affected: %d", email, rowsAffected)
        http.Redirect(w, r, "/", http.StatusSeeOther)
    }
}

func serveDeleteAccount(w http.ResponseWriter, r *http.Request) {
    session, _ := store.Get(r, "session")
    username, ok := session.Values["user_id"].(string)
    if !ok || username == "" {
        http.Redirect(w, r, "/login", http.StatusSeeOther)
        return
    }

    // Delete the user's account from the database
    _, err := db.Exec("DELETE FROM users WHERE username = ?", username)
    if err != nil {
        log.Printf("Error deleting user account: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Clear the session
    session.Options.MaxAge = -1
    err = session.Save(r, w)
    if err != nil {
        log.Printf("Error clearing session: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    log.Printf("User %s deleted their account successfully", username)
    http.Redirect(w, r, "/", http.StatusSeeOther)
}

func serveForgotUsername(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        http.ServeFile(w, r, "forgotUsername.html")
        return
    }

    if r.Method == http.MethodPost {
        email := r.FormValue("email")

        var username string
        err := db.QueryRow("SELECT username FROM users WHERE email = ?", email).Scan(&username)
        if err != nil {
            log.Printf("Error querying user from database: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        // Send email with username
        sendUsernameEmail(email, username)

        log.Printf("Username reminder sent to %s", email)
        http.Redirect(w, r, "/login", http.StatusSeeOther)
    }
}

func serveForgotPassword(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        http.ServeFile(w, r, "forgotPassword.html")
        return
    }

    if r.Method == http.MethodPost {
        username := r.FormValue("username")
        email := r.FormValue("email")

        var dbUsername, dbEmail string
        err := db.QueryRow("SELECT username, email FROM users WHERE username = ? AND email = ?", username, email).Scan(&dbUsername, &dbEmail)
        if err != nil {
            if err == sql.ErrNoRows {
                log.Printf("No matching user found for username: %s and email: %s", username, email)
                http.Error(w, "Invalid username or email", http.StatusUnauthorized)
            } else {
                log.Printf("Error querying user from database: %v", err)
                http.Error(w, "Internal server error", http.StatusInternalServerError)
            }
            return
        }

        // Send email with reset password link
        sendResetPasswordEmail(email, username)

        log.Printf("Password reset link sent to %s", email)
        http.Redirect(w, r, "/login", http.StatusSeeOther)
    }
}

func serveResetPassword(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
        http.ServeFile(w, r, "resetPassword.html")
        return
    }

    if r.Method == http.MethodPost {
        username := r.FormValue("username")
        password := r.FormValue("password")
        confirmPassword := r.FormValue("confirmPassword")

        if password != confirmPassword {
            http.Error(w, "Passwords do not match", http.StatusBadRequest)
            return
        }

        passwordHash, err := hashPassword(password)
        if err != nil {
            log.Printf("Error hashing password: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        _, err = db.Exec("UPDATE users SET password_hash = ? WHERE username = ?", passwordHash, username)
        if err != nil {
            log.Printf("Error updating user in database: %v", err)
            http.Error(w, "Internal server error", http.StatusInternalServerError)
            return
        }

        log.Printf("Password for user %s reset successfully", username)
        http.Redirect(w, r, "/login", http.StatusSeeOther)
    }
}

func serveContact(w http.ResponseWriter, r *http.Request) {
    if r.Method == http.MethodGet {
    
        permissions := Permissions{
            IsAuthenticated: isAuthenticated(r),
            IsAdmin:         hasRole(r, "admin"),
        }

        err := contactTemplate.ExecuteTemplate(w, "contact", permissions)
        if err != nil {
            http.Error(w, err.Error(), http.StatusInternalServerError)
        }
        return
    }

    if r.Method == http.MethodPost {
        name := r.FormValue("name")
        email := r.FormValue("email")
        message := r.FormValue("message")

        // Validate that all fields are filled
        if name == "" || email == "" || message == "" {
            http.Error(w, "All fields are required", http.StatusBadRequest)
            return
        }

        // Send the email
        err := sendContactEmail(name, email, message)
        if err != nil {
            log.Printf("Error sending contact email: %v", err)
            http.Error(w, "Failed to send message. Please try again later.", http.StatusInternalServerError)
            return
        }

        log.Printf("Contact form submitted by %s (%s)", name, email)
        http.Redirect(w, r, "/", http.StatusSeeOther)
    }
}

func sendUsernameEmail(email, username string) {
    smtpHost := os.Getenv("SMTP_HOST")
    smtpPort := os.Getenv("SMTP_PORT")
    smtpUser := os.Getenv("SMTP_USER")
    smtpPass := os.Getenv("SMTP_PASS")

    auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

    from := smtpUser
    subject := "Your Username"
    body := fmt.Sprintf("Your username is: %s", username)

    msg := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\n\n%s", from, email, subject, body)

    err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, []string{email}, []byte(msg))
    if err != nil {
        log.Printf("Error sending email: %v", err)
    }
}

func sendResetPasswordEmail(email, username string) {
    smtpHost := os.Getenv("SMTP_HOST")
    smtpPort := os.Getenv("SMTP_PORT")
    smtpUser := os.Getenv("SMTP_USER")
    smtpPass := os.Getenv("SMTP_PASS")

    auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

    from := smtpUser
    subject := "Reset Your Password"
    body := fmt.Sprintf(`
        <html>
        <body>
        <p>Click the link to reset your password:</p>
        <p><a href="http://localhost:8080/resetPassword?username=%s">Reset Password</a></p>
        </body>
        </html>
    `, username)

    msg := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\nMIME-Version: 1.0\nContent-Type: text/html; charset=\"UTF-8\"\nContent-Transfer-Encoding: 7bit\n\n%s", from, email, subject, body)

    err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, []string{email}, []byte(msg))
    if err != nil {
        log.Printf("Error sending email: %v", err)
    }
}

func sendContactEmail(name, email, message string) error {
    smtpHost := os.Getenv("SMTP_HOST")
    smtpPort := os.Getenv("SMTP_PORT")
    smtpUser := os.Getenv("SMTP_USER")
    smtpPass := os.Getenv("SMTP_PASS")

    auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

    to := "kdz@kepa.com" // Replace with the email address where you want to receive messages
    subject := "Dev World Contact Form Submission"
    body := fmt.Sprintf(`
        <html>
        <body>
        <h2>New Contact Form Submission</h2>
        <p><strong>Name:</strong> %s</p>
        <p><strong>Email:</strong> %s</p>
        <p><strong>Message:</strong></p>
        <p>%s</p>
        </body>
        </html>
    `, name, email, message)

    msg := fmt.Sprintf("From: %s\nTo: %s\nSubject: %s\nMIME-Version: 1.0\nContent-Type: text/html; charset=\"UTF-8\"\n\n%s", email, to, subject, body)

    return smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{to}, []byte(msg))
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