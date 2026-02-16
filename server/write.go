package main

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	thingscloud "github.com/arthursoares/things-cloud-sdk"
)

// ---------------------------------------------------------------------------
// Wire-format types (no omitempty — Things expects all fields on creates)
// ---------------------------------------------------------------------------

type wireNote struct {
	TypeTag  string `json:"_t"`
	Checksum int64  `json:"ch"`
	Value    string `json:"v"`
	Type     int    `json:"t"`
}

type wireExtension struct {
	Sn      map[string]any `json:"sn"`
	TypeTag string         `json:"_t"`
}

type writeEnvelope struct {
	id      string
	action  int
	kind    string
	payload any
}

func (w writeEnvelope) UUID() string { return w.id }

func (w writeEnvelope) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		T int    `json:"t"`
		E string `json:"e"`
		P any    `json:"p"`
	}{w.action, w.kind, w.payload})
}

type taskCreatePayload struct {
	Tp   int              `json:"tp"`
	Sr   *int64           `json:"sr"`
	Dds  *int64           `json:"dds"`
	Rt   []string         `json:"rt"`
	Rmd  *int64           `json:"rmd"`
	Ss   int              `json:"ss"`
	Tr   bool             `json:"tr"`
	Dl   []string         `json:"dl"`
	Icp  bool             `json:"icp"`
	St   int              `json:"st"`
	Ar   []string         `json:"ar"`
	Tt   string           `json:"tt"`
	Do   int              `json:"do"`
	Lai  *int64           `json:"lai"`
	Tir  *int64           `json:"tir"`
	Tg   []string         `json:"tg"`
	Agr  []string         `json:"agr"`
	Ix   int              `json:"ix"`
	Cd   float64          `json:"cd"`
	Lt   bool             `json:"lt"`
	Icc  int              `json:"icc"`
	Md   *float64         `json:"md"`
	Ti   int              `json:"ti"`
	Dd   *int64           `json:"dd"`
	Ato  *int             `json:"ato"`
	Nt   wireNote         `json:"nt"`
	Icsd *int64           `json:"icsd"`
	Pr   []string         `json:"pr"`
	Rp   *string          `json:"rp"`
	Acrd *int64           `json:"acrd"`
	Sp   *float64         `json:"sp"`
	Sb   int              `json:"sb"`
	Rr   *json.RawMessage `json:"rr"`
	Xx   wireExtension    `json:"xx"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func emptyNote() wireNote {
	return wireNote{TypeTag: "tx", Checksum: 0, Value: "", Type: 1}
}

func noteChecksum(s string) int64 {
	return int64(crc32.ChecksumIEEE([]byte(s)))
}

func textNote(s string) wireNote {
	return wireNote{TypeTag: "tx", Checksum: noteChecksum(s), Value: s, Type: 1}
}

func defaultExtension() wireExtension {
	return wireExtension{Sn: map[string]any{}, TypeTag: "oo"}
}

func generateUUID() string {
	u := uuid.New()
	const alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	n := new(big.Int).SetBytes(u[:])
	base := big.NewInt(58)
	mod := new(big.Int)
	var encoded []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		encoded = append(encoded, alphabet[mod.Int64()])
	}
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}
	return string(encoded)
}

func nowTs() float64 {
	return float64(time.Now().UnixNano()) / 1e9
}

func todayMidnightUTC() int64 {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Unix()
}

// ---------------------------------------------------------------------------
// Fluent update builder
// ---------------------------------------------------------------------------

type taskUpdate struct {
	fields map[string]any
}

func newTaskUpdate() *taskUpdate {
	return &taskUpdate{fields: map[string]any{
		"md": nowTs(),
	}}
}

func (u *taskUpdate) title(s string) *taskUpdate {
	u.fields["tt"] = s
	return u
}

func (u *taskUpdate) note(text string) *taskUpdate {
	u.fields["nt"] = textNote(text)
	return u
}

func (u *taskUpdate) clearNote() *taskUpdate {
	u.fields["nt"] = emptyNote()
	return u
}

func (u *taskUpdate) status(ss int) *taskUpdate {
	u.fields["ss"] = ss
	return u
}

func (u *taskUpdate) stopDate(ts float64) *taskUpdate {
	u.fields["sp"] = ts
	return u
}

func (u *taskUpdate) trash(b bool) *taskUpdate {
	u.fields["tr"] = b
	return u
}

func (u *taskUpdate) schedule(st int, sr, tir any) *taskUpdate {
	u.fields["st"] = st
	u.fields["sr"] = sr
	u.fields["tir"] = tir
	return u
}

func (u *taskUpdate) deadline(dd int64) *taskUpdate {
	u.fields["dd"] = dd
	return u
}

func (u *taskUpdate) clearDeadline() *taskUpdate {
	u.fields["dd"] = nil
	return u
}

func (u *taskUpdate) project(uuid string) *taskUpdate {
	u.fields["pr"] = []string{uuid}
	return u
}

func (u *taskUpdate) tags(uuids []string) *taskUpdate {
	u.fields["tg"] = uuids
	return u
}

func (u *taskUpdate) build() map[string]any {
	return u.fields
}

// ---------------------------------------------------------------------------
// API request types
// ---------------------------------------------------------------------------

// CreateTaskRequest is the JSON body for POST /api/tasks/create.
type CreateTaskRequest struct {
	Title    string `json:"title"`
	Note     string `json:"note,omitempty"`
	When     string `json:"when,omitempty"`     // today, anytime, someday, inbox
	Deadline string `json:"deadline,omitempty"` // YYYY-MM-DD
	Project  string `json:"project,omitempty"`  // project UUID
	Tags     string `json:"tags,omitempty"`     // comma-separated tag UUIDs
}

// EditTaskRequest is the JSON body for POST /api/tasks/edit.
type EditTaskRequest struct {
	UUID     string `json:"uuid"`
	Title    string `json:"title,omitempty"`
	Note     string `json:"note,omitempty"`
	When     string `json:"when,omitempty"`
	Deadline string `json:"deadline,omitempty"`
	Project  string `json:"project,omitempty"`
	Tags     string `json:"tags,omitempty"`
}

// UUIDRequest is the JSON body for complete/trash endpoints.
type UUIDRequest struct {
	UUID string `json:"uuid"`
}

// ---------------------------------------------------------------------------
// Core write operations (used by both HTTP handlers and MCP tools)
// ---------------------------------------------------------------------------

// historyWrite syncs the history to get the latest ancestor index, then writes.
// If the write still fails with 409 (race with Things app), it retries once.
func historyWrite(env writeEnvelope) error {
	if err := history.Sync(); err != nil {
		return fmt.Errorf("history sync failed: %w", err)
	}
	err := history.Write(env)
	if err != nil && strings.Contains(err.Error(), "409") {
		// Retry once — another client may have committed between our sync and write
		if err2 := history.Sync(); err2 != nil {
			return fmt.Errorf("history re-sync failed: %w", err2)
		}
		err = history.Write(env)
	}
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

// parseWhen interprets the when parameter. Returns (st, sr, tir, handled).
// For named values (today/anytime/someday/inbox/none) and YYYY-MM-DD dates.
// A future date goes to Upcoming (st=2), today's date goes to Today (st=1).
func parseWhen(when string) (st int, sr, tir *int64, handled bool) {
	switch when {
	case "today":
		today := todayMidnightUTC()
		return 1, &today, &today, true
	case "anytime":
		return 1, nil, nil, true
	case "someday":
		return 2, nil, nil, true
	case "inbox":
		return 0, nil, nil, true
	case "none", "":
		return -1, nil, nil, false
	default:
		// Try parsing as YYYY-MM-DD
		if t, err := time.Parse("2006-01-02", when); err == nil {
			ts := t.UTC().Unix()
			today := todayMidnightUTC()
			if ts <= today {
				// Today or past → Today view
				return 1, &ts, &ts, true
			}
			// Future → Upcoming view (st=2 with date)
			return 2, &ts, nil, true
		}
		return -1, nil, nil, false
	}
}

func createTask(req CreateTaskRequest) (string, error) {
	taskUUID := generateUUID()
	now := nowTs()

	var st int
	var sr, tir *int64
	var dd *int64

	if s, r, t, ok := parseWhen(req.When); ok {
		st, sr, tir = s, r, t
	} else {
		st = 0 // inbox
	}

	if req.Deadline != "" {
		if t, err := time.Parse("2006-01-02", req.Deadline); err == nil {
			ts := t.Unix()
			dd = &ts
		}
	}

	pr := []string{}
	if req.Project != "" {
		pr = []string{req.Project}
		if req.When == "" {
			st = 1
		}
	}

	tg := []string{}
	if req.Tags != "" {
		tg = strings.Split(req.Tags, ",")
	}

	nt := emptyNote()
	if req.Note != "" {
		nt = textNote(req.Note)
	}

	payload := taskCreatePayload{
		Tp: 0, Sr: sr, Dds: nil, Rt: []string{}, Rmd: nil,
		Ss: 0, Tr: false, Dl: []string{}, Icp: false, St: st,
		Ar: []string{}, Tt: req.Title, Do: 0, Lai: nil, Tir: tir,
		Tg: tg, Agr: []string{}, Ix: 0, Cd: now, Lt: false,
		Icc: 0, Md: nil, Ti: 0, Dd: dd, Ato: nil, Nt: nt,
		Icsd: nil, Pr: pr, Rp: nil, Acrd: nil, Sp: nil,
		Sb: 0, Rr: nil, Xx: defaultExtension(),
	}

	env := writeEnvelope{id: taskUUID, action: 0, kind: "Task6", payload: payload}
	if err := historyWrite(env); err != nil {
		return "", err
	}
	syncer.Sync()
	return taskUUID, nil
}

func completeTask(uuid string) error {
	ts := nowTs()
	u := newTaskUpdate().status(3).stopDate(ts)
	env := writeEnvelope{id: uuid, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func trashTask(uuid string) error {
	u := newTaskUpdate().trash(true)
	env := writeEnvelope{id: uuid, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func editTask(req EditTaskRequest) error {
	u := newTaskUpdate()
	if req.Title != "" {
		u.title(req.Title)
	}
	if req.Note != "" {
		u.note(req.Note)
	}
	if req.When == "none" {
		u.fields["sr"] = nil
		u.fields["tir"] = nil
	} else if st, sr, tir, ok := parseWhen(req.When); ok {
		u.schedule(st, sr, tir)
	}
	if req.Deadline == "none" {
		u.clearDeadline()
	} else if req.Deadline != "" {
		if t, err := time.Parse("2006-01-02", req.Deadline); err == nil {
			u.deadline(t.Unix())
		}
	}
	if req.Project != "" {
		u.project(req.Project)
		if req.When == "" {
			u.schedule(1, 0, 0)
		}
	}
	if req.Tags != "" {
		u.tags(strings.Split(req.Tags, ","))
	}
	env := writeEnvelope{id: req.UUID, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func moveTaskToToday(uuid string) error {
	today := todayMidnightUTC()
	u := newTaskUpdate().schedule(1, today, today)
	env := writeEnvelope{id: uuid, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func moveTaskToAnytime(uuid string) error {
	u := newTaskUpdate().schedule(1, nil, nil)
	env := writeEnvelope{id: uuid, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func moveTaskToSomeday(uuid string) error {
	u := newTaskUpdate().schedule(2, nil, nil)
	env := writeEnvelope{id: uuid, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func moveTaskToInbox(uuid string) error {
	u := newTaskUpdate().schedule(0, nil, nil)
	env := writeEnvelope{id: uuid, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func uncompleteTask(uuid string) error {
	u := newTaskUpdate().status(0).stopDate(0)
	env := writeEnvelope{id: uuid, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func untrashTask(uuid string) error {
	u := newTaskUpdate().trash(false)
	env := writeEnvelope{id: uuid, action: 1, kind: "Task6", payload: u.build()}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func createArea(title string, tagUUIDs []string) (string, error) {
	areaUUID := generateUUID()
	if tagUUIDs == nil {
		tagUUIDs = []string{}
	}
	payload := map[string]any{
		"ix": 0,
		"tt": title,
		"tg": tagUUIDs,
	}
	env := writeEnvelope{id: areaUUID, action: 0, kind: "Area3", payload: payload}
	if err := historyWrite(env); err != nil {
		return "", err
	}
	syncer.Sync()
	return areaUUID, nil
}

func createTag(title, shorthand, parentUUID string) (string, error) {
	tagUUID := generateUUID()
	pn := []string{}
	if parentUUID != "" {
		pn = []string{parentUUID}
	}
	payload := map[string]any{
		"ix": 0,
		"tt": title,
		"sh": shorthand,
		"pn": pn,
	}
	env := writeEnvelope{id: tagUUID, action: 0, kind: "Tag4", payload: payload}
	if err := historyWrite(env); err != nil {
		return "", err
	}
	syncer.Sync()
	return tagUUID, nil
}

func createHeading(title, projectUUID string) (string, error) {
	headingUUID := generateUUID()
	now := nowTs()

	pr := []string{}
	if projectUUID != "" {
		pr = []string{projectUUID}
	}

	payload := taskCreatePayload{
		Tp: 2, Sr: nil, Dds: nil, Rt: []string{}, Rmd: nil,
		Ss: 0, Tr: false, Dl: []string{}, Icp: false, St: 1,
		Ar: []string{}, Tt: title, Do: 0, Lai: nil, Tir: nil,
		Tg: []string{}, Agr: []string{}, Ix: 0, Cd: now, Lt: false,
		Icc: 0, Md: nil, Ti: 0, Dd: nil, Ato: nil, Nt: emptyNote(),
		Icsd: nil, Pr: pr, Rp: nil, Acrd: nil, Sp: nil,
		Sb: 0, Rr: nil, Xx: defaultExtension(),
	}

	env := writeEnvelope{id: headingUUID, action: 0, kind: "Task6", payload: payload}
	if err := historyWrite(env); err != nil {
		return "", err
	}
	syncer.Sync()
	return headingUUID, nil
}

func createProject(title, note, when, deadline, areaUUID string) (string, error) {
	projectUUID := generateUUID()
	now := nowTs()

	var st int
	var sr, tir *int64
	var dd *int64

	switch when {
	case "today":
		st = 1
		today := todayMidnightUTC()
		sr = &today
		tir = &today
	case "someday":
		st = 2
	default:
		st = 1 // projects default to anytime
	}

	if deadline != "" {
		if t, err := time.Parse("2006-01-02", deadline); err == nil {
			ts := t.Unix()
			dd = &ts
		}
	}

	ar := []string{}
	if areaUUID != "" {
		ar = []string{areaUUID}
	}

	nt := emptyNote()
	if note != "" {
		nt = textNote(note)
	}

	payload := taskCreatePayload{
		Tp: 1, Sr: sr, Dds: nil, Rt: []string{}, Rmd: nil,
		Ss: 0, Tr: false, Dl: []string{}, Icp: false, St: st,
		Ar: ar, Tt: title, Do: 0, Lai: nil, Tir: tir,
		Tg: []string{}, Agr: []string{}, Ix: 0, Cd: now, Lt: false,
		Icc: 0, Md: nil, Ti: 0, Dd: dd, Ato: nil, Nt: nt,
		Icsd: nil, Pr: []string{}, Rp: nil, Acrd: nil, Sp: nil,
		Sb: 0, Rr: nil, Xx: defaultExtension(),
	}

	env := writeEnvelope{id: projectUUID, action: 0, kind: "Task6", payload: payload}
	if err := historyWrite(env); err != nil {
		return "", err
	}
	syncer.Sync()
	return projectUUID, nil
}

// ---------------------------------------------------------------------------
// Checklist item operations
// ---------------------------------------------------------------------------

func createChecklistItem(title, taskUUID string) (string, error) {
	itemUUID := generateUUID()
	now := nowTs()
	payload := map[string]any{
		"tt": title,
		"ts": []string{taskUUID},
		"ix": 0,
		"cd": now,
		"md": nil,
		"ss": 0,
		"sp": nil,
		"lt": false,
		"xx": defaultExtension(),
	}
	env := writeEnvelope{id: itemUUID, action: 0, kind: "ChecklistItem3", payload: payload}
	if err := historyWrite(env); err != nil {
		return "", err
	}
	syncer.Sync()
	return itemUUID, nil
}

func completeChecklistItem(uuid string) error {
	ts := nowTs()
	payload := map[string]any{
		"md": ts,
		"ss": 3,
		"sp": ts,
	}
	env := writeEnvelope{id: uuid, action: 1, kind: "ChecklistItem3", payload: payload}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func uncompleteChecklistItem(uuid string) error {
	payload := map[string]any{
		"md": nowTs(),
		"ss": 0,
		"sp": nil,
	}
	env := writeEnvelope{id: uuid, action: 1, kind: "ChecklistItem3", payload: payload}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

func deleteChecklistItem(uuid string) error {
	// Delete via Tombstone2
	tombUUID := generateUUID()
	payload := map[string]any{
		"dloid": uuid,
		"dld":   nowTs(),
	}
	env := writeEnvelope{id: tombUUID, action: 0, kind: "Tombstone2", payload: payload}
	if err := historyWrite(env); err != nil {
		return err
	}
	syncer.Sync()
	return nil
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}
	if req.Title == "" {
		jsonError(w, "title is required", 400)
		return
	}
	taskUUID, err := createTask(req)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]string{"status": "created", "uuid": taskUUID, "title": req.Title})
}

func handleCompleteTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req UUIDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}
	if req.UUID == "" {
		jsonError(w, "uuid is required", 400)
		return
	}
	if err := completeTask(req.UUID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]string{"status": "completed", "uuid": req.UUID})
}

func handleTrashTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req UUIDRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}
	if req.UUID == "" {
		jsonError(w, "uuid is required", 400)
		return
	}
	if err := trashTask(req.UUID); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]string{"status": "trashed", "uuid": req.UUID})
}

func handleEditTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", 405)
		return
	}
	var req EditTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid JSON: "+err.Error(), 400)
		return
	}
	if req.UUID == "" {
		jsonError(w, "uuid is required", 400)
		return
	}
	if err := editTask(req); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonResponse(w, map[string]string{"status": "updated", "uuid": req.UUID})
}

// Ensure writeEnvelope implements Identifiable
var _ thingscloud.Identifiable = writeEnvelope{}
