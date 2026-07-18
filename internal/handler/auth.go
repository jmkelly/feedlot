package handler

import (
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
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
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
	existing, err := h.DB.GetUserByEmail(email)
	if err != nil {
		log.Printf("check existing user: %v", err)
	}
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
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
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
		Secure:   r.TLS != nil,
	})

	// HTMX redirect; fall back to 303 for non-HTMX clients
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/login")
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func (h *Handler) renderAuth(w http.ResponseWriter, r *http.Request, page, email, errorMsg string) {
	data := map[string]any{
		"Page":  page,
		"Email": email,
		"Error": errorMsg,
	}

	if err := authTmpl.Execute(w, data); err != nil {
		log.Printf("render auth template: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

const authPageTemplate = `
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Feedlot — {{if eq .Page "login"}}Sign in{{else}}Register{{end}}</title>
  <script>(function(){var t=null;try{t=localStorage.getItem('feedlot:theme')}catch(e){}if(t!=='light'&&t!=='dark'){t=window.matchMedia('(prefers-color-scheme: dark)').matches?'dark':'light'}document.documentElement.setAttribute('data-theme',t)})();</script>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=Bricolage+Grotesque:opsz,wght@12..96,400;12..96,600;12..96,700&family=JetBrains+Mono:wght@400;600&family=Newsreader:ital,opsz,wght@0,6..72,400..700;1,6..72,400..600&display=swap" rel="stylesheet">
  <script src="https://unpkg.com/htmx.org@2"></script>
  <script src="/static/js/app.js" defer></script>
  <link rel="icon" type="image/svg+xml" href="/static/favicon.svg">
  <link rel="stylesheet" href="/static/css/app.css">
</head>
<body class="authwrap">
  <div class="authcard">
    <div class="authhero">
      <div class="authhero__mark">🐄</div>
      <h1 class="authhero__name">Feedlot</h1>
      <p class="authhero__sub">Chew through your feeds, one article at a time.</p>
    </div>
    <div class="authform">
      {{if .Error}}<div class="alert alert--err">{{.Error}}</div>{{end}}
      {{if eq .Page "login"}}
      <form hx-post="/login" hx-target="body" hx-swap="outerHTML">
        <h2>Sign in</h2>
        <div class="field">
          <label for="email">Email</label>
          <input type="email" id="email" name="email" value="{{.Email}}" required class="input" autocomplete="email" autofocus>
        </div>
        <div class="field">
          <label for="password">Password</label>
          <div class="password-wrap">
            <input type="password" id="password" name="password" required class="input" autocomplete="current-password">
            <button type="button" class="password-toggle" aria-label="Show password" onclick="togglePasswordVisibility(this, 'password')">👁</button>
          </div>
        </div>
        <button type="submit" class="btn btn--primary w-full">Sign in</button>
        <p class="authswitch">Don't have an account? <a href="/register">Register</a></p>
      </form>
      {{else}}
      <form hx-post="/register" hx-target="body" hx-swap="outerHTML">
        <h2>Create account</h2>
        <div class="field">
          <label for="email">Email</label>
          <input type="email" id="email" name="email" value="{{.Email}}" required class="input" autocomplete="email" autofocus>
        </div>
        <div class="field">
          <label for="password">Password</label>
          <div class="password-wrap">
            <input type="password" id="password" name="password" required minlength="8" class="input" autocomplete="new-password">
            <button type="button" class="password-toggle" aria-label="Show password" onclick="togglePasswordVisibility(this, 'password')">👁</button>
          </div>
          <p class="field-hint">At least 8 characters</p>
        </div>
        <div class="field">
          <label for="confirm_password">Confirm password</label>
          <div class="password-wrap">
            <input type="password" id="confirm_password" name="confirm_password" required minlength="8" class="input" autocomplete="new-password">
            <button type="button" class="password-toggle" aria-label="Show password" onclick="togglePasswordVisibility(this, 'confirm_password')">👁</button>
          </div>
        </div>
        <button type="submit" class="btn btn--primary w-full">Create account</button>
        <p class="authswitch">Already have an account? <a href="/login">Sign in</a></p>
      </form>
      {{end}}
    </div>
  </div>
</body>
</html>
`
