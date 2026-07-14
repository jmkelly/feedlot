package handler

import (
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/james/feedlot/internal/auth"
)

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.renderAuth(w, r, "login", "", "")
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderAuth(w, r, "login", "", "Invalid form data")
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	if email == "" || password == "" {
		h.renderAuth(w, r, "login", "", "Email and password are required")
		return
	}

	user, err := h.DB.GetUserByEmail(email)
	if err != nil {
		h.renderAuth(w, r, "login", email, "Invalid email or password")
		return
	}

	if !h.Auth.VerifyPassword(password, user.PasswordHash) {
		h.renderAuth(w, r, "login", email, "Invalid email or password")
		return
	}

	sessionToken, err := auth.GenerateSessionToken()
	if err != nil {
		log.Printf("generate session token: %v", err)
		h.renderAuth(w, r, "login", email, "Server error, please try again")
		return
	}

	jwtToken, expiresAt, err := h.Auth.GenerateToken(user.ID)
	if err != nil {
		log.Printf("generate jwt: %v", err)
		h.renderAuth(w, r, "login", email, "Server error, please try again")
		return
	}

	_, err = h.DB.CreateSession(user.ID, sessionToken, expiresAt)
	if err != nil {
		log.Printf("create session: %v", err)
		h.renderAuth(w, r, "login", email, "Server error, please try again")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "feedlot_token",
		Value:    jwtToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	h.renderAuth(w, r, "register", "", "")
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderAuth(w, r, "register", "", "Invalid form data")
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")

	if email == "" || password == "" {
		h.renderAuth(w, r, "register", email, "Email and password are required")
		return
	}

	if password != confirm {
		h.renderAuth(w, r, "register", email, "Passwords do not match")
		return
	}

	if len(password) < 8 {
		h.renderAuth(w, r, "register", email, "Password must be at least 8 characters")
		return
	}

	// Check if email already exists
	existing, _ := h.DB.GetUserByEmail(email)
	if existing != nil {
		h.renderAuth(w, r, "register", email, "Email already registered")
		return
	}

	hash, err := h.Auth.HashPassword(password)
	if err != nil {
		log.Printf("hash password: %v", err)
		h.renderAuth(w, r, "register", email, "Server error, please try again")
		return
	}

	user, err := h.DB.CreateUser(email, hash)
	if err != nil {
		log.Printf("create user: %v", err)
		h.renderAuth(w, r, "register", email, "Server error, please try again")
		return
	}

	sessionToken, err := auth.GenerateSessionToken()
	if err != nil {
		log.Printf("generate session token: %v", err)
		h.renderAuth(w, r, "register", email, "Server error, please try again")
		return
	}

	jwtToken, expiresAt, err := h.Auth.GenerateToken(user.ID)
	if err != nil {
		log.Printf("generate jwt: %v", err)
		h.renderAuth(w, r, "register", email, "Server error, please try again")
		return
	}

	_, err = h.DB.CreateSession(user.ID, sessionToken, expiresAt)
	if err != nil {
		log.Printf("create session: %v", err)
		h.renderAuth(w, r, "register", email, "Server error, please try again")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "feedlot_token",
		Value:    jwtToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	userID := GetUserID(r)

	// Delete all user sessions
	if userID > 0 {
		_ = h.DB.DeleteUserSessions(userID)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "feedlot_token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// HTMX redirect
	w.Header().Set("HX-Redirect", "/login")
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) renderAuth(w http.ResponseWriter, r *http.Request, page, email, errorMsg string) {
	data := map[string]any{
		"Page":  page,
		"Email": email,
		"Error": errorMsg,
	}

	tmpl := template.Must(template.New("auth").Parse(authPageTemplate))
	if err := tmpl.Execute(w, data); err != nil {
		log.Printf("render auth template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

const authPageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — {{if eq .Page "login"}}Login{{else}}Register{{end}}</title>
  <script src="https://unpkg.com/htmx.org@2"></script>
  <link rel="stylesheet" href="/static/css/app.css">
</head>
<body class="bg-stone-50 text-stone-900 antialiased min-h-screen flex items-center justify-center">
  <div class="w-full max-w-md mx-4">
    <div class="text-center mb-8">
      <h1 class="text-4xl font-bold text-amber-800">🐄 Feedlot</h1>
      <p class="text-stone-500 mt-2">Chew through your feeds, one article at a time.</p>
    </div>
    <div class="bg-white rounded-xl shadow-sm border border-stone-200 p-6">
      {{if .Error}}<div class="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded-lg mb-4 text-sm">{{.Error}}</div>{{end}}
      {{if eq .Page "login"}}
      <form hx-post="/login" hx-target="body" hx-swap="outerHTML" class="space-y-4">
        <h2 class="text-xl font-semibold text-stone-800 mb-4">Sign in</h2>
        <div>
          <label for="email" class="block text-sm font-medium text-stone-700 mb-1">Email</label>
          <input type="email" id="email" name="email" value="{{.Email}}" required
            class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
        </div>
        <div>
          <label for="password" class="block text-sm font-medium text-stone-700 mb-1">Password</label>
          <input type="password" id="password" name="password" required
            class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
        </div>
        <button type="submit" class="w-full bg-amber-600 hover:bg-amber-700 text-white font-medium py-2 px-4 rounded-lg transition-colors">
          Sign in
        </button>
        <p class="text-center text-sm text-stone-500 mt-4">
          Don't have an account? <a href="/register" class="text-amber-600 hover:text-amber-700 font-medium">Register</a>
        </p>
      </form>
      {{else}}
      <form hx-post="/register" hx-target="body" hx-swap="outerHTML" class="space-y-4">
        <h2 class="text-xl font-semibold text-stone-800 mb-4">Create account</h2>
        <div>
          <label for="email" class="block text-sm font-medium text-stone-700 mb-1">Email</label>
          <input type="email" id="email" name="email" value="{{.Email}}" required
            class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
        </div>
        <div>
          <label for="password" class="block text-sm font-medium text-stone-700 mb-1">Password</label>
          <input type="password" id="password" name="password" required minlength="8"
            class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
        </div>
        <div>
          <label for="confirm_password" class="block text-sm font-medium text-stone-700 mb-1">Confirm password</label>
          <input type="password" id="confirm_password" name="confirm_password" required minlength="8"
            class="w-full px-3 py-2 border border-stone-300 rounded-lg focus:ring-2 focus:ring-amber-500 focus:border-amber-500 outline-none">
        </div>
        <button type="submit" class="w-full bg-amber-600 hover:bg-amber-700 text-white font-medium py-2 px-4 rounded-lg transition-colors">
          Create account
        </button>
        <p class="text-center text-sm text-stone-500 mt-4">
          Already have an account? <a href="/login" class="text-amber-600 hover:text-amber-700 font-medium">Sign in</a>
        </p>
      </form>
      {{end}}
    </div>
  </div>
</body>
</html>`
