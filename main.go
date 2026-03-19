package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var version = "dev"

const defaultAPIURL = "https://api.docstash.dev"

type authConfig struct {
	APIURL      string `json:"api_url"`
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	// Per-command help
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		printCommandHelp(cmd)
		return
	}

	switch cmd {
	case "login":
		runLogin(args)
	case "logout":
		runLogout()
	case "me":
		runMe(args)
	case "list":
		runList(args)
	case "search":
		runSearch(args)
	case "get":
		runGet(args)
	case "create":
		runCreate(args)
	case "update":
		runUpdate(args)
	case "delete":
		runDelete(args)
	case "tags":
		runTags(args)
	case "tag":
		runTag(args)
	case "edit":
		runEdit(args)
	case "help", "--help", "-h":
		if len(args) > 0 {
			printCommandHelp(args[0])
		} else {
			printUsage()
		}
	case "version", "--version", "-v":
		fmt.Printf("docstash %s\n", version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`docstash %s — CLI for DocStash, an AI-first document store.

Usage: docstash <command> [options]

Commands:
  login                                Sign in via GitHub or Google
  logout                               Remove stored credentials
  me                                   Show current user

  list [--tag TAG] [--limit N]         List documents
  search QUERY [--tag TAG] [--limit N] Full-text search
  get ID                               Get document with full content

  create --title TITLE [options]       Create document (pipe content via stdin)
  update ID [--title T] [--summary S]  Update document (pipe content via stdin)
  delete ID                            Delete document
  edit ID --old TEXT --new TEXT         Find-and-replace edit

  tags                                 List all tags with counts
  tag ID tag1,tag2,...                 Set tags on a document

Global options:
  --api-url URL    API base URL (default: %s, or set DOCSTASH_API_URL)
  --json           Output raw JSON instead of formatted text

Run "docstash <command> --help" for details on a specific command.

Examples:
  docstash login
  docstash list --tag research
  docstash search "kubernetes setup"
  docstash get 550e8400-e29b-41d4-a716-446655440000
  echo "# My notes" | docstash create --title "Notes" --tags notes,draft
  echo "Updated content" | docstash update 550e8400 --title "New title"
  docstash edit 550e8400 --old "old text" --new "new text"
  docstash tag 550e8400 research,important
  docstash delete 550e8400
`, version, defaultAPIURL)
}

var commandHelp = map[string]string{
	"login": `Usage: docstash login [--api-url URL]

Authenticate with DocStash via GitHub or Google OAuth.
Opens your browser to sign in. The session token is stored locally at
~/.config/docstash/auth.json and expires after 1 hour.

Options:
  --api-url URL    API base URL (default: ` + defaultAPIURL + `)

Examples:
  docstash login
  docstash login --api-url http://localhost:8080
`,

	"logout": `Usage: docstash logout

Remove stored credentials from ~/.config/docstash/auth.json.
`,

	"me": `Usage: docstash me [--json]

Show the currently authenticated user (name, email, ID).
`,

	"list": `Usage: docstash list [--tag TAG] [--limit N] [--json]

List your documents (without content). Sorted by last updated.

Options:
  --tag TAG      Filter by tag
  --limit N      Max results (default 20, max 100)
  --json         Output raw JSON

Examples:
  docstash list
  docstash list --tag research --limit 5
`,

	"search": `Usage: docstash search QUERY [--tag TAG] [--limit N] [--json]

Full-text search across document titles, summaries, and content.
Results are ranked by relevance.

Arguments:
  QUERY          Search query (required)

Options:
  --tag TAG      Filter by tag
  --limit N      Max results (default 20, max 100)
  --json         Output raw JSON

Examples:
  docstash search "kubernetes deployment"
  docstash search "API design" --tag architecture
`,

	"get": `Usage: docstash get ID [--json]

Get a document by ID, including its full content, tags, and metadata.

Arguments:
  ID             Document UUID (required)

Examples:
  docstash get 550e8400-e29b-41d4-a716-446655440000
  docstash get 550e8400-e29b-41d4-a716-446655440000 --json
`,

	"create": `Usage: docstash create --title TITLE [--summary S] [--tags t1,t2] [--json] [< content.md]

Create a new document. Content can be piped via stdin.

Options:
  --title TITLE  Document title (required)
  --summary S    Short description
  --tags t1,t2   Comma-separated tags
  --json         Output raw JSON

Examples:
  docstash create --title "Meeting Notes" --tags meetings,2026
  echo "# Design Doc" | docstash create --title "Design" --summary "System design"
  cat notes.md | docstash create --title "Notes" --tags notes
`,

	"update": `Usage: docstash update ID [--title T] [--summary S] [--json] [< content.md]

Update an existing document. Only provided fields are changed.
Pipe new content via stdin to replace the document content.

Arguments:
  ID             Document UUID (required)

Options:
  --title T      New title
  --summary S    New summary
  --json         Output raw JSON

Examples:
  docstash update 550e8400 --title "New Title"
  cat updated.md | docstash update 550e8400
  echo "new content" | docstash update 550e8400 --title "Also new title"
`,

	"delete": `Usage: docstash delete ID [--json]

Delete a document permanently.

Arguments:
  ID             Document UUID (required)

Examples:
  docstash delete 550e8400-e29b-41d4-a716-446655440000
`,

	"edit": `Usage: docstash edit ID --old TEXT --new TEXT [--json]

Edit a document's content using find-and-replace. The old text must appear
exactly once in the document content.

Arguments:
  ID             Document UUID (required)

Options:
  --old TEXT     Text to find (required, must match exactly once)
  --new TEXT     Replacement text (use "" to delete)
  --json         Output raw JSON

Examples:
  docstash edit 550e8400 --old "draft version" --new "final version"
  docstash edit 550e8400 --old "remove this line" --new ""
`,

	"tags": `Usage: docstash tags [--json]

List all tags across your documents with document counts.

Examples:
  docstash tags
  docstash tags --json
`,

	"tag": `Usage: docstash tag ID tag1,tag2,... [--json]

Set tags on a document. Replaces all existing tags.

Arguments:
  ID             Document UUID (required)
  TAGS           Comma-separated list of tags (required)

Examples:
  docstash tag 550e8400 research,important,draft
  docstash tag 550e8400 archive
`,
}

func printCommandHelp(cmd string) {
	if help, ok := commandHelp[cmd]; ok {
		fmt.Print(help)
	} else {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

// --- Auth ---

func authPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "docstash", "auth.json")
}

func loadAuth() *authConfig {
	data, err := os.ReadFile(authPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "Not logged in. Run: docstash login")
		os.Exit(1)
	}
	var cfg authConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "Corrupt auth file. Run: docstash login")
		os.Exit(1)
	}
	if cfg.ExpiresAt != "" {
		exp, err := time.Parse(time.RFC3339, cfg.ExpiresAt)
		if err == nil && time.Now().After(exp) {
			fmt.Fprintln(os.Stderr, "Session expired. Run: docstash login")
			os.Exit(1)
		}
	}
	return &cfg
}

func saveAuth(cfg *authConfig) {
	dir := filepath.Dir(authPath())
	os.MkdirAll(dir, 0700)
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(authPath(), data, 0600); err != nil {
		fatal("Failed to save auth: %v", err)
	}
}

// --- API helpers ---

func getAPIURL(args []string) string {
	for i, a := range args {
		if a == "--api-url" && i+1 < len(args) {
			return args[i+1]
		}
	}
	if v := os.Getenv("DOCSTASH_API_URL"); v != "" {
		return v
	}
	return defaultAPIURL
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func getFlagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func apiRequest(cfg *authConfig, method, path string, body any) (map[string]any, int) {
	var bodyReader io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		bodyReader = strings.NewReader(string(data))
	}

	req, err := http.NewRequest(method, cfg.APIURL+path, bodyReader)
	if err != nil {
		fatal("Request error: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+cfg.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fatal("Connection error: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(respBody, &result)
	return result, resp.StatusCode
}

func requireOK(result map[string]any, status int) {
	if status >= 400 {
		msg := "request failed"
		if detail, ok := result["detail"].(string); ok {
			msg = detail
		} else if title, ok := result["title"].(string); ok {
			msg = title
		}
		fatal("Error (%d): %s", status, msg)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func stdinContent() string {
	info, _ := os.Stdin.Stat()
	if info.Mode()&os.ModeCharDevice == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err == nil && len(data) > 0 {
			return string(data)
		}
	}
	return ""
}

// --- Login (OAuth via browser) ---

func runLogin(args []string) {
	apiURL := getAPIURL(args)

	// Start local callback server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fatal("Failed to start local server: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Register OAuth client
	regBody, _ := json.Marshal(map[string]any{
		"redirect_uris": []string{callbackURL},
		"client_name":   "docstash-cli",
	})
	regResp, err := http.Post(apiURL+"/oauth/register", "application/json", strings.NewReader(string(regBody)))
	if err != nil {
		fatal("Failed to register client: %v", err)
	}
	defer regResp.Body.Close()
	var regResult map[string]any
	json.NewDecoder(regResp.Body).Decode(&regResult)
	clientID, ok := regResult["client_id"].(string)
	if !ok {
		fatal("Failed to register OAuth client")
	}

	// Generate PKCE
	verifierBytes := make([]byte, 32)
	rand.Read(verifierBytes)
	codeVerifier := hex.EncodeToString(verifierBytes)
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	// Generate state
	stateBytes := make([]byte, 16)
	rand.Read(stateBytes)
	state := hex.EncodeToString(stateBytes)

	// Build authorize URL
	authorizeURL := apiURL + "/oauth/authorize?" + url.Values{
		"client_id":             {clientID},
		"redirect_uri":         {callbackURL},
		"code_challenge":       {codeChallenge},
		"code_challenge_method": {"S256"},
		"response_type":        {"code"},
		"state":                {state},
	}.Encode()

	// Channel for the auth code
	codeCh := make(chan string, 1)
	errCh := make(chan string, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		returnedState := r.URL.Query().Get("state")
		if returnedState != state {
			w.WriteHeader(400)
			fmt.Fprint(w, "State mismatch. Please try again.")
			errCh <- "state mismatch"
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			w.WriteHeader(400)
			fmt.Fprint(w, "No authorization code received.")
			errCh <- "no code"
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;margin:0"><div style="text-align:center"><h1>Logged in!</h1><p>You can close this tab and return to your terminal.</p></div></body></html>`)
		codeCh <- code
	})

	server := &http.Server{Handler: mux}
	go server.Serve(listener)

	fmt.Println("Opening browser to sign in...")
	openBrowser(authorizeURL)
	fmt.Printf("If the browser didn't open, visit:\n%s\n\nWaiting for authentication...\n", authorizeURL)

	// Wait for callback
	var code string
	select {
	case code = <-codeCh:
	case e := <-errCh:
		server.Close()
		fatal("Authentication failed: %s", e)
	case <-time.After(5 * time.Minute):
		server.Close()
		fatal("Authentication timed out")
	}
	server.Close()

	// Exchange code for token
	tokenResp, err := http.PostForm(apiURL+"/oauth/token", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {clientID},
		"redirect_uri":  {callbackURL},
		"code_verifier": {codeVerifier},
	})
	if err != nil {
		fatal("Token exchange failed: %v", err)
	}
	defer tokenResp.Body.Close()
	var tokenResult map[string]any
	json.NewDecoder(tokenResp.Body).Decode(&tokenResult)

	accessToken, ok := tokenResult["access_token"].(string)
	if !ok {
		fatal("Failed to get access token")
	}
	expiresIn, _ := tokenResult["expires_in"].(float64)
	if expiresIn == 0 {
		expiresIn = 3600
	}
	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)

	saveAuth(&authConfig{
		APIURL:      apiURL,
		AccessToken: accessToken,
		ExpiresAt:   expiresAt.Format(time.RFC3339),
	})

	fmt.Println("Logged in successfully!")
}

func runLogout() {
	if err := os.Remove(authPath()); err != nil && !os.IsNotExist(err) {
		fatal("Failed to remove auth: %v", err)
	}
	fmt.Println("Logged out.")
}

// --- Commands ---

func runMe(args []string) {
	cfg := loadAuth()
	result, status := apiRequest(cfg, "GET", "/api/v1/auth/me", nil)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	fmt.Printf("Name:  %s\nEmail: %s\nID:    %s\n",
		strVal(result, "name"), strVal(result, "email"), strVal(result, "id"))
}

func runList(args []string) {
	cfg := loadAuth()
	params := url.Values{}
	if v := getFlagValue(args, "--tag"); v != "" {
		params.Set("tag", v)
	}
	if v := getFlagValue(args, "--limit"); v != "" {
		params.Set("limit", v)
	}
	path := "/api/v1/documents"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	result, status := apiRequest(cfg, "GET", path, nil)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	printDocList(result)
}

func runSearch(args []string) {
	cfg := loadAuth()
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fatal("Usage: docstash search QUERY [--tag TAG] [--limit N]")
	}
	query := args[0]
	params := url.Values{"q": {query}}
	if v := getFlagValue(args, "--tag"); v != "" {
		params.Set("tag", v)
	}
	if v := getFlagValue(args, "--limit"); v != "" {
		params.Set("limit", v)
	}
	result, status := apiRequest(cfg, "GET", "/api/v1/documents/search?"+params.Encode(), nil)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	printDocList(result)
}

func runGet(args []string) {
	cfg := loadAuth()
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fatal("Usage: docstash get ID")
	}
	id := args[0]
	result, status := apiRequest(cfg, "GET", "/api/v1/documents/"+id, nil)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	printDoc(result)
}

func runCreate(args []string) {
	cfg := loadAuth()
	title := getFlagValue(args, "--title")
	if title == "" {
		fatal("Usage: docstash create --title TITLE [--summary S] [--tags t1,t2] [< content.md]")
	}

	body := map[string]any{"title": title}
	if v := getFlagValue(args, "--summary"); v != "" {
		body["summary"] = v
	}
	if v := getFlagValue(args, "--tags"); v != "" {
		body["tags"] = strings.Split(v, ",")
	}
	if content := stdinContent(); content != "" {
		body["content"] = content
	}

	result, status := apiRequest(cfg, "POST", "/api/v1/documents", body)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	fmt.Printf("Created: %s (%s)\n", strVal(result, "title"), strVal(result, "id"))
}

func runUpdate(args []string) {
	cfg := loadAuth()
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fatal("Usage: docstash update ID [--title T] [--summary S] [< content.md]")
	}
	id := args[0]

	body := map[string]any{}
	if v := getFlagValue(args, "--title"); v != "" {
		body["title"] = v
	}
	if v := getFlagValue(args, "--summary"); v != "" {
		body["summary"] = v
	}
	if content := stdinContent(); content != "" {
		body["content"] = content
	}
	if len(body) == 0 {
		fatal("Nothing to update. Provide --title, --summary, or pipe content via stdin.")
	}

	result, status := apiRequest(cfg, "PUT", "/api/v1/documents/"+id, body)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	fmt.Printf("Updated: %s (%s)\n", strVal(result, "title"), strVal(result, "id"))
}

func runDelete(args []string) {
	cfg := loadAuth()
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fatal("Usage: docstash delete ID")
	}
	id := args[0]
	result, status := apiRequest(cfg, "DELETE", "/api/v1/documents/"+id, nil)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	fmt.Println("Deleted.")
}

func runTags(args []string) {
	cfg := loadAuth()
	result, status := apiRequest(cfg, "GET", "/api/v1/tags", nil)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	tags, _ := result["tags"].([]any)
	if len(tags) == 0 {
		fmt.Println("No tags.")
		return
	}
	for _, t := range tags {
		tag, _ := t.(map[string]any)
		count, _ := tag["count"].(float64)
		fmt.Printf("  %-20s %d documents\n", strVal(tag, "tag"), int(count))
	}
}

func runTag(args []string) {
	cfg := loadAuth()
	if len(args) < 2 {
		fatal("Usage: docstash tag ID tag1,tag2,...")
	}
	id := args[0]
	tags := strings.Split(args[1], ",")

	body := map[string]any{"tags": tags}
	result, status := apiRequest(cfg, "PUT", "/api/v1/documents/"+id+"/tags", body)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	fmt.Printf("Tags set: %s\n", strings.Join(tags, ", "))
}

func runEdit(args []string) {
	cfg := loadAuth()
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fatal("Usage: docstash edit ID --old TEXT --new TEXT")
	}
	id := args[0]
	oldStr := getFlagValue(args, "--old")
	newStr := getFlagValue(args, "--new")
	if oldStr == "" {
		fatal("--old is required")
	}

	body := map[string]any{
		"edits": []map[string]any{
			{"old_string": oldStr, "new_string": newStr},
		},
	}
	result, status := apiRequest(cfg, "PATCH", "/api/v1/documents/"+id, body)
	requireOK(result, status)

	if hasFlag(args, "--json") {
		printJSON(result)
		return
	}
	fmt.Printf("Edited: %s\n", strVal(result, "title"))
}

// --- Output helpers ---

func printJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func printDocList(result map[string]any) {
	docs, _ := result["documents"].([]any)
	if len(docs) == 0 {
		fmt.Println("No documents found.")
		return
	}
	for _, d := range docs {
		doc, _ := d.(map[string]any)
		id := strVal(doc, "id")
		if len(id) > 8 {
			id = id[:8]
		}
		title := strVal(doc, "title")
		tags := formatTags(doc)
		updated := formatTime(strVal(doc, "updated_at"))
		if tags != "" {
			fmt.Printf("  %s  %-40s  [%s]  %s\n", id, title, tags, updated)
		} else {
			fmt.Printf("  %s  %-40s  %s\n", id, title, updated)
		}
	}
	if cursor, ok := result["next_cursor"].(string); ok && cursor != "" {
		fmt.Printf("\n  More results available (cursor: %s)\n", cursor)
	}
}

func printDoc(doc map[string]any) {
	fmt.Printf("# %s\n", strVal(doc, "title"))
	fmt.Printf("ID: %s", strVal(doc, "id"))
	if tags := formatTags(doc); tags != "" {
		fmt.Printf("  |  Tags: %s", tags)
	}
	fmt.Printf("  |  Updated: %s\n", formatTime(strVal(doc, "updated_at")))
	if summary := strVal(doc, "summary"); summary != "" {
		fmt.Printf("Summary: %s\n", summary)
	}
	fmt.Println(strings.Repeat("─", 60))
	fmt.Println(strVal(doc, "content"))
}

func strVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func formatTags(doc map[string]any) string {
	tags, ok := doc["tags"].([]any)
	if !ok || len(tags) == 0 {
		return ""
	}
	strs := make([]string, 0, len(tags))
	for _, t := range tags {
		if s, ok := t.(string); ok {
			strs = append(strs, s)
		}
	}
	return strings.Join(strs, ", ")
}

func formatTime(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.Local().Format("Jan 02 15:04")
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		cmd.Start()
	}
}
