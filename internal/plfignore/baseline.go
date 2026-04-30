package plfignore

// baselinePatterns are compiled into every Promptcellar build and run on
// every prompt by default. They cover well-known secret shapes so a team
// gets reasonable protection without authoring a `.promptcellarignore`.
//
// Match overrides:
//
//   - `.promptcellarallow` (same syntax as `.promptcellarignore`) lets a
//     team whitelist substrings the baseline incorrectly flagged — typically
//     fixture/placeholder values in docs or test data.
//
//   - `.promptcellarignore` is authoritative: a team's deny rule always wins
//     over the baseline AND over allow.
//
// Pattern IDs match `[A-Za-z0-9_-]+` per the PLF schema (so they fit in
// `excluded.pattern_id`). Naming convention: lowercase-kebab, vendor-prefixed
// where applicable. They are stable contracts — renaming an ID is a breaking
// change for anyone bucket-counting against `pattern_id`.
//
// Each entry has a short Description used by `pc-cli doctor` and the README
// reference table.
var baselinePatterns = []rawPattern{
	// ─── Cloud: AWS ────────────────────────────────────────────────────────
	{
		ID:          "aws-access-key-id",
		Regex:       `\b(?:AKIA|ASIA|AGPA|AROA|AIDA|ANPA|ANVA|ASCA)[0-9A-Z]{16}\b`,
		Description: "AWS access-key ID (any type — IAM user, STS, role, etc.)",
	},
	{
		ID:          "aws-secret-access-key-assignment",
		Regex:       `(?i)aws[_-]?(?:secret[_-]?access[_-]?key|secret)\s*[=:]\s*[A-Za-z0-9/+=]{40}`,
		Description: "AWS secret-access-key assigned via env-style or YAML/JSON key",
	},

	// ─── Cloud: GCP ────────────────────────────────────────────────────────
	{
		ID:          "gcp-api-key",
		Regex:       `\bAIza[0-9A-Za-z_-]{35}\b`,
		Description: "Google Cloud API key (Maps, Firebase, generic GCP)",
	},
	{
		ID:          "gcp-oauth-token",
		Regex:       `\bya29\.[0-9A-Za-z_-]{20,}\b`,
		Description: "Google OAuth 2.0 access token",
	},
	{
		ID:          "gcp-service-account-json",
		Regex:       `"type"\s*:\s*"service_account"`,
		Description: "GCP service-account JSON key (recognised by the type marker)",
	},

	// ─── Source forges: GitHub / GitLab ────────────────────────────────────
	{
		ID:          "github-pat-classic",
		Regex:       `\bghp_[A-Za-z0-9]{36,}\b`,
		Description: "GitHub classic personal access token",
	},
	{
		ID:          "github-pat-fine-grained",
		Regex:       `\bgithub_pat_[A-Za-z0-9_]{82}\b`,
		Description: "GitHub fine-grained personal access token",
	},
	{
		ID:          "github-oauth",
		Regex:       `\bgho_[A-Za-z0-9]{36,}\b`,
		Description: "GitHub OAuth token (web flow)",
	},
	{
		ID:          "github-app-user-token",
		Regex:       `\bghu_[A-Za-z0-9]{36,}\b`,
		Description: "GitHub App user-to-server token",
	},
	{
		ID:          "github-app-server-token",
		Regex:       `\bghs_[A-Za-z0-9]{36,}\b`,
		Description: "GitHub App server-to-server token",
	},
	{
		ID:          "github-refresh-token",
		Regex:       `\bghr_[A-Za-z0-9]{36,}\b`,
		Description: "GitHub OAuth refresh token",
	},
	{
		ID:          "gitlab-pat",
		Regex:       `\bglpat-[A-Za-z0-9_-]{20,}\b`,
		Description: "GitLab personal access token",
	},

	// ─── AI / LLM providers ────────────────────────────────────────────────
	{
		ID:          "anthropic-api-key",
		Regex:       `\bsk-ant-(?:api|admin)\d{2}-[A-Za-z0-9_-]{40,}\b`,
		Description: "Anthropic API key (sk-ant-api##- or sk-ant-admin##-)",
	},
	{
		ID:          "openai-api-key",
		Regex:       `\bsk-(?:proj-|svcacct-|None-)?[A-Za-z0-9_-]{32,}\b`,
		Description: "OpenAI API key (legacy, project, or service-account scoped)",
	},

	// ─── Payment ───────────────────────────────────────────────────────────
	{
		ID:          "stripe-secret-live",
		Regex:       `\bsk_live_[A-Za-z0-9]{24,}\b`,
		Description: "Stripe live secret key",
	},
	{
		ID:          "stripe-secret-test",
		Regex:       `\bsk_test_[A-Za-z0-9]{24,}\b`,
		Description: "Stripe test secret key",
	},
	{
		ID:          "stripe-restricted",
		Regex:       `\brk_(?:live|test)_[A-Za-z0-9]{24,}\b`,
		Description: "Stripe restricted key",
	},
	{
		ID:          "stripe-publishable-live",
		Regex:       `\bpk_live_[A-Za-z0-9]{24,}\b`,
		Description: "Stripe live publishable key (rate-limited but identifying)",
	},

	// ─── Messaging / communication ─────────────────────────────────────────
	{
		ID:          "slack-token",
		Regex:       `\bxox[baprs]-[A-Za-z0-9-]{10,}\b`,
		Description: "Slack token (bot/app/OAuth/refresh/user)",
	},
	{
		ID:          "slack-webhook",
		Regex:       `\bhttps://hooks\.slack\.com/services/T[A-Za-z0-9]+/B[A-Za-z0-9]+/[A-Za-z0-9]+\b`,
		Description: "Slack incoming-webhook URL",
	},
	{
		ID:          "discord-bot-token",
		Regex:       `\b[MN][A-Za-z0-9_-]{23,}\.[A-Za-z0-9_-]{6}\.[A-Za-z0-9_-]{27,}\b`,
		Description: "Discord bot token (three dot-separated base64url-safe segments)",
	},
	{
		ID:          "twilio-api-key",
		Regex:       `\bSK[a-f0-9]{32}\b`,
		Description: "Twilio API key SID",
	},
	{
		ID:          "twilio-account-sid",
		Regex:       `\bAC[a-f0-9]{32}\b`,
		Description: "Twilio account SID (paired with the auth token; both are sensitive)",
	},
	{
		ID:          "sendgrid-api-key",
		Regex:       `\bSG\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{40,}\b`,
		Description: "SendGrid API key",
	},

	// ─── Generic JWT ───────────────────────────────────────────────────────
	{
		ID:          "jwt",
		Regex:       `\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b`,
		Description: "JSON Web Token (three base64url-safe segments separated by dots)",
	},

	// ─── PEM / SSH private keys ───────────────────────────────────────────
	{
		ID:          "private-key-pem",
		Regex:       `-----BEGIN (?:RSA |DSA |EC |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY( BLOCK)?-----`,
		Description: "PEM-armoured private key header (RSA/DSA/EC/OpenSSH/PGP/encrypted)",
	},

	// ─── DB / message-queue connection strings ────────────────────────────
	{
		ID:          "db-url-with-password",
		Regex:       `(?i)\b(?:postgres(?:ql)?|mysql|mariadb|mongodb(?:\+srv)?|redis|amqp|rabbitmq)://[^\s:@/]+:[^\s:@/]+@`,
		Description: "Database / message-queue connection URL with embedded credentials",
	},

	// ─── npm / PyPI ───────────────────────────────────────────────────────
	{
		ID:          "npm-token",
		Regex:       `\bnpm_[A-Za-z0-9]{36,}\b`,
		Description: "npm authentication token",
	},
	{
		ID:          "pypi-token",
		Regex:       `\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_-]{50,}\b`,
		Description: "PyPI API token",
	},

	// ─── Other common SaaS ─────────────────────────────────────────────────
	{
		ID:          "mailgun-api-key",
		Regex:       `\bkey-[a-f0-9]{32}\b`,
		Description: "Mailgun API key",
	},
	{
		ID:          "mailchimp-api-key",
		Regex:       `\b[a-f0-9]{32}-us[0-9]{1,2}\b`,
		Description: "MailChimp API key (32-hex with -us<dc> datacenter suffix)",
	},
	{
		ID:          "datadog-api-key-assignment",
		Regex:       `(?i)\bdd[_-]?api[_-]?key\b\s*[=:]\s*['"]?[a-f0-9]{32}['"]?`,
		Description: "Datadog API key assigned to a DD_API_KEY-shaped variable",
	},
	{
		ID:          "heroku-api-key-assignment",
		Regex:       `(?i)\bheroku[_-]?(?:api[_-]?)?key\b\s*[=:]\s*['"]?[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}['"]?`,
		Description: "Heroku API key (UUID assigned to a heroku_api_key-shaped variable)",
	},

	// ─── Generic env-style assignment (catch-all) ─────────────────────────
	// Important: this is intentionally narrow — it requires both a known
	// secret-like name AND a token-shaped value of ≥16 chars. Plain
	// `password = "hello"` won't trigger.
	{
		ID:          "generic-secret-assignment",
		Regex:       `(?i)\b(?:api[_-]?key|api[_-]?secret|access[_-]?token|auth[_-]?token|secret[_-]?key|client[_-]?secret|private[_-]?key|password|passwd|pwd)\b\s*[=:]\s*['"]?[A-Za-z0-9_/+=.\-]{16,}['"]?`,
		Description: "Generic SECRET=… / API_KEY=… style assignment with a token-shaped value",
	},
}

type rawPattern struct {
	ID          string
	Regex       string
	Description string
}

// BaselineDescriptions returns (id, description) pairs for every built-in
// pattern, in declaration order. Used by `pc-cli doctor` and tooling that
// wants to enumerate the active baseline.
func BaselineDescriptions() []struct{ ID, Description string } {
	out := make([]struct{ ID, Description string }, 0, len(baselinePatterns))
	for _, p := range baselinePatterns {
		out = append(out, struct{ ID, Description string }{p.ID, p.Description})
	}
	return out
}
