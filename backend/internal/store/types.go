package store

import (
	"encoding/json"
	"time"
)

// Rows mirror the tables in migrations/. JSON names are the column names.
// omitzero leaves zero values out of inserts so the database defaults apply.

type Workspace struct {
	ID        string         `json:"id"`
	UserID    string         `json:"user_id"` // owner
	Name      string         `json:"name"`
	Timezone  string         `json:"timezone"`
	Locale    string         `json:"locale"`
	Plan      string         `json:"plan"`
	Profile   map[string]any `json:"profile"` // About me facts
	Needs     []string       `json:"needs"`   // onboarding chips (template categories), at most 3
	CreatedAt time.Time      `json:"created_at,omitzero"`
	UpdatedAt time.Time      `json:"updated_at,omitzero"`
}

// NewWorkspace is the input to CreateWorkspace.
type NewWorkspace struct {
	OwnerID  string
	Name     string
	Timezone string // IANA name; "" = Europe/Berlin
	Locale   string // en or de; "" = en
}

type WorkspacePatch struct {
	Name     *string        `json:"name,omitempty"`
	Timezone *string        `json:"timezone,omitempty"`
	Locale   *string        `json:"locale,omitempty"`
	Profile  map[string]any `json:"profile,omitempty"` // replaces the whole profile
	Needs    *[]string      `json:"needs,omitempty"`   // replaces the list
}

// Member roles.
const (
	RoleOwner  = "owner"
	RoleMember = "member"
)

type Member struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	UserID      string    `json:"user_id"`
	Role        string    `json:"role"`
	DisplayName *string   `json:"display_name,omitempty"`
	AboutMe     *string   `json:"about_me,omitempty"`
	Signature   *string   `json:"signature,omitempty"`
	CreatedAt   time.Time `json:"created_at,omitzero"`
}

type Helper struct {
	ID               string    `json:"id"`
	WorkspaceID      string    `json:"workspace_id"`
	UserID           string    `json:"user_id"`
	Name             string    `json:"name"`
	Avatar           *string   `json:"avatar,omitempty"`
	Color            *string   `json:"color,omitempty"`
	Status           string    `json:"status,omitzero"` // draft, active, paused
	CurrentVersionID *string   `json:"current_version_id,omitempty"`
	TemplateID       *string   `json:"template_id,omitempty"`
	CreatedVia       string    `json:"created_via,omitzero"`
	CreatedAt        time.Time `json:"created_at,omitzero"`
	UpdatedAt        time.Time `json:"updated_at,omitzero"`
}

type HelperPatch struct {
	Name   *string `json:"name,omitempty"`
	Status *string `json:"status,omitempty"` // active, paused
}

type HelperVersion struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspace_id"`
	UserID       string          `json:"user_id"`
	HelperID     string          `json:"helper_id"`
	Version      int             `json:"version"`
	Stack        json.RawMessage `json:"stack"`
	SourcePrompt *string         `json:"source_prompt,omitempty"`
	CreatedBy    *string         `json:"created_by,omitempty"`
	CreatedAt    time.Time       `json:"created_at,omitzero"`
}

// Trigger types.
const (
	TriggerSchedule = "schedule"
	TriggerManual   = "manual"
	TriggerEmail    = "email"
	TriggerWatch    = "watch"
	TriggerWebhook  = "webhook"
)

// Trigger is what starts a helper's runs (scheduler, inbound email, …).
type Trigger struct {
	ID             string         `json:"id"`
	WorkspaceID    string         `json:"workspace_id"`
	UserID         string         `json:"user_id"`
	HelperID       string         `json:"helper_id"`
	Type           string         `json:"type"`
	Config         map[string]any `json:"config,omitzero"`
	NextRunAt      *time.Time     `json:"next_run_at,omitempty"`
	InboundAddress *string        `json:"inbound_address,omitempty"`
	WebhookToken   *string        `json:"webhook_token,omitempty"`
	CreatedAt      time.Time      `json:"created_at,omitzero"`
	UpdatedAt      time.Time      `json:"updated_at,omitzero"`
}

type TriggerPatch struct {
	Config    map[string]any       `json:"config,omitempty"`
	NextRunAt *Nullable[time.Time] `json:"next_run_at,omitempty"`
}

// Run statuses.
const (
	RunQueued  = "queued"
	RunRunning = "running"
	RunWaiting = "waiting"
	RunDone    = "done"
	RunFailed  = "failed"
	RunStopped = "stopped"
)

type Run struct {
	ID              string         `json:"id"`
	WorkspaceID     string         `json:"workspace_id"`
	UserID          string         `json:"user_id"`
	HelperID        string         `json:"helper_id"`
	VersionID       string         `json:"version_id"`
	TriggerType     string         `json:"trigger_type"` // schedule, manual, email, watch, webhook, test
	TriggerData     map[string]any `json:"trigger_data,omitzero"`
	TestMode        bool           `json:"test_mode,omitzero"`
	Status          string         `json:"status,omitzero"`
	NextBrickIndex  int            `json:"next_brick_index,omitzero"`
	WaitingOnCardID *string        `json:"waiting_on_card_id,omitempty"`
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	FinishedAt      *time.Time     `json:"finished_at,omitempty"`
	CostEUR         Num            `json:"cost_eur,omitzero"`
	TokensIn        int            `json:"tokens_in,omitzero"`
	TokensOut       int            `json:"tokens_out,omitzero"`
	Error           *string        `json:"error,omitempty"`
	CreatedAt       time.Time      `json:"created_at,omitzero"`
}

type RunPatch struct {
	Status          *string           `json:"status,omitempty"`
	NextBrickIndex  *int              `json:"next_brick_index,omitempty"`
	WaitingOnCardID *Nullable[string] `json:"waiting_on_card_id,omitempty"`
	StartedAt       *time.Time        `json:"started_at,omitempty"`
	FinishedAt      *time.Time        `json:"finished_at,omitempty"`
	CostEUR         *Num              `json:"cost_eur,omitempty"`
	TokensIn        *int              `json:"tokens_in,omitempty"`
	TokensOut       *int              `json:"tokens_out,omitempty"`
	Error           *Nullable[string] `json:"error,omitempty"`
}

type RunFilter struct {
	WorkspaceID string
	HelperID    string
	Status      string
	Limit       int // 0 = all
}

type Step struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspace_id"`
	UserID      string           `json:"user_id"`
	RunID       string           `json:"run_id"`
	BrickID     string           `json:"brick_id"`   // id inside the stack, e.g. "b3"
	BrickType   string           `json:"brick_type"` // catalog id, e.g. "look.read_page"
	Position    int              `json:"position"`
	Attempt     int              `json:"attempt,omitzero"` // default 1
	Status      string           `json:"status,omitzero"`  // default running
	CardsIn     int              `json:"cards_in,omitzero"`
	CardsOut    int              `json:"cards_out,omitzero"`
	StartedAt   time.Time        `json:"started_at,omitzero"`
	FinishedAt  *time.Time       `json:"finished_at,omitempty"`
	CostEUR     Num              `json:"cost_eur,omitzero"`
	Log         []map[string]any `json:"log,omitzero"`
}

type StepPatch struct {
	Status     *string          `json:"status,omitempty"`
	CardsIn    *int             `json:"cards_in,omitempty"`
	CardsOut   *int             `json:"cards_out,omitempty"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
	CostEUR    *Num             `json:"cost_eur,omitempty"`
	Log        []map[string]any `json:"log,omitempty"`
}

// Card is one snapshot of a logical card (CardID) in one step's pile.
type Card struct {
	ID          string            `json:"id"`
	WorkspaceID string            `json:"workspace_id"`
	UserID      string            `json:"user_id"`
	RunID       string            `json:"run_id"`
	StepID      string            `json:"step_id"`
	CardID      string            `json:"card_id"`
	ParentID    *string           `json:"parent_id,omitempty"`
	Position    int               `json:"position"` // always sent; insert_cards would otherwise use input order
	Kind        string            `json:"kind"`
	Data        map[string]any    `json:"data,omitzero"`
	Trust       map[string]string `json:"trust,omitzero"` // field → user_entered | extracted | untrusted
	SourceURL   *string           `json:"source_url,omitempty"`
	Kept        *bool             `json:"kept,omitempty"`
	Score       *Num              `json:"score,omitempty"`
	Reason      *string           `json:"reason,omitempty"`
	CreatedAt   time.Time         `json:"created_at,omitzero"`
}

type FeedCard struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	UserID      string         `json:"user_id"`
	HelperID    *string        `json:"helper_id,omitempty"`
	RunID       *string        `json:"run_id,omitempty"`
	StepID      *string        `json:"step_id,omitempty"`
	Type        string         `json:"type"` // result, approval, pick, question, problem
	Title       string         `json:"title"`
	Body        *string        `json:"body,omitempty"`
	Payload     map[string]any `json:"payload,omitzero"`
	State       string         `json:"state,omitzero"` // default open
	Decision    map[string]any `json:"decision,omitzero"`
	ExpiresAt   *time.Time     `json:"expires_at,omitempty"`
	DecidedBy   *string        `json:"decided_by,omitempty"`
	DecidedAt   *time.Time     `json:"decided_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at,omitzero"`
}

// FeedFilter selects Feed cards. Empty Types/States mean any.
type FeedFilter struct {
	WorkspaceID string
	Types       []string
	States      []string
	NotStates   []string
	Before      *time.Time // created_at before (paging)
	Limit       int        // 0 = all
}

type FeedCardPatch struct {
	State     *string        `json:"state,omitempty"`
	Decision  map[string]any `json:"decision,omitempty"`
	DecidedBy *string        `json:"decided_by,omitempty"`
	DecidedAt *time.Time     `json:"decided_at,omitempty"`
	ExpiresAt *time.Time     `json:"expires_at,omitempty"`
}

type List struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspace_id"`
	UserID      string           `json:"user_id"`
	Name        string           `json:"name"`
	Columns     []map[string]any `json:"columns,omitzero"`
	DedupeField *string          `json:"dedupe_field,omitempty"`
	ShareToken  *string          `json:"share_token,omitempty"`
	CreatedAt   time.Time        `json:"created_at,omitzero"`
	UpdatedAt   time.Time        `json:"updated_at,omitzero"`
}

type ListRow struct {
	ID          string         `json:"id"`
	WorkspaceID string         `json:"workspace_id"`
	UserID      string         `json:"user_id"`
	ListID      string         `json:"list_id"`
	Data        map[string]any `json:"data,omitzero"`
	DedupeKey   *string        `json:"dedupe_key,omitempty"`
	SourceRunID *string        `json:"source_run_id,omitempty"`
	CreatedAt   time.Time      `json:"created_at,omitzero"`
}

type UsageEntry struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspace_id"`
	UserID       string    `json:"user_id"`
	RunID        *string   `json:"run_id,omitempty"`
	Kind         string    `json:"kind"`                 // llm, search, fetch, email, places
	BrainMode    *string   `json:"brain_mode,omitempty"` // included, byok
	Model        *string   `json:"model,omitempty"`
	Units        Num       `json:"units,omitzero"`
	BrainCredits Num       `json:"brain_credits,omitzero"`
	EstCostEUR   Num       `json:"est_cost_eur,omitzero"`
	CreatedAt    time.Time `json:"created_at,omitzero"`
}

// Place is where helpers tell the user things (a Discord or Slack webhook,
// a Telegram chat, Email me, Posta).
type Place struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Type        string `json:"type"` // discord, slack, teams, telegram, email, posta
	Label       string `json:"label"`
	// SecretCiphertext: base64 of an internal/crypto envelope sealed for
	// crypto.AAD("places", id, "secret"). Never returned to the app.
	SecretCiphertext *string        `json:"secret_ciphertext,omitempty"`
	SecretLast4      *string        `json:"secret_last4,omitempty"`
	Meta             map[string]any `json:"meta,omitzero"`
	VerifiedAt       *time.Time     `json:"verified_at,omitempty"`
	CreatedAt        time.Time      `json:"created_at,omitzero"`
}

type PlacePatch struct {
	Label      *string    `json:"label,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
}

// Brain is a workspace's AI: the included model with a monthly allowance,
// or (post-MVP) the user's own key.
type Brain struct {
	ID                    string    `json:"id"`
	WorkspaceID           string    `json:"workspace_id"`
	UserID                string    `json:"user_id"`
	Mode                  string    `json:"mode"`   // included, byok
	Status                string    `json:"status"` // ok, invalid, over_budget, allowance_used
	AllowanceCredits      Num       `json:"allowance_credits"`
	UsedCreditsThisPeriod Num       `json:"used_credits_this_period"`
	PeriodStart           time.Time `json:"period_start"`
	CreatedAt             time.Time `json:"created_at,omitzero"`
}

// CreditsLeft is what's left of the included allowance this month (a new
// month counts as a fresh allowance even before the first usage resets it).
func (b Brain) CreditsLeft(now time.Time) float64 {
	used := float64(b.UsedCreditsThisPeriod)
	month := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	if b.PeriodStart.Before(month) {
		used = 0
	}
	return float64(b.AllowanceCredits) - used
}

type OutboundMessage struct {
	ID                string     `json:"id"`
	WorkspaceID       string     `json:"workspace_id"`
	UserID            string     `json:"user_id"`
	RunID             *string    `json:"run_id,omitempty"`
	IdempotencyKey    string     `json:"idempotency_key"` // run_id + brick_id + card_id
	Channel           string     `json:"channel"`
	Lane              *int       `json:"lane,omitempty"` // email only: 1, 2 or 3
	Recipient         string     `json:"recipient"`
	Subject           *string    `json:"subject,omitempty"`
	Body              *string    `json:"body,omitempty"`
	MediaFileIDs      []string   `json:"media_file_ids,omitzero"`
	ScheduledFor      *time.Time `json:"scheduled_for,omitempty"`
	Status            string     `json:"status,omitzero"` // default pending
	ProviderMessageID *string    `json:"provider_message_id,omitempty"`
	Error             *string    `json:"error,omitempty"`
	SentAt            *time.Time `json:"sent_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at,omitzero"`
}

type OutboundPatch struct {
	Status            *string    `json:"status,omitempty"`
	ProviderMessageID *string    `json:"provider_message_id,omitempty"`
	Error             *string    `json:"error,omitempty"`
	SentAt            *time.Time `json:"sent_at,omitempty"`
}
