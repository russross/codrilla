package main

import (
	"encoding/json"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

func init() {
	r := pat.New()
	r.Add("GET", `/problem/types`, handlerInstructor(problem_types))
	r.Add("GET", `/problem/type/{tag:[\w:]+$}`, handlerInstructor(problem_type))
	r.Add("POST", `/problem/new`, handlerInstructorJson(problem_new))
	r.Add("POST", `/problem/update/{id:\d+$}`, handlerInstructorJson(problem_update))
	r.Add("GET", `/problem/get/{id:\d+$}`, handlerInstructor(problem_get))
	r.Add("GET", `/problem/tags`, handlerInstructor(problem_tags))
	http.Handle("/problem/", r)
}

func validProblemTag(s string) bool {
	if len(s) < 1 {
		return false
	}
	for _, ch := range s {
		if !unicode.IsLower(ch) && !unicode.IsDigit(ch) && !strings.ContainsRune("-_", ch) {
			return false
		}
	}
	return true
}

type ProblemField struct {
	Name    string
	Prompt  string
	Title   string
	Type    string
	List    bool
	Default string
	Creator string
	Student string
	Grader  string
	Result  string
}

type ProblemType struct {
	Name      string
	Tag       string
	FieldList []ProblemField
}

func setupProblemTypes(db *redis.Client) {
	// first get the list of problem types from the grader
	u := &url.URL{
		Scheme: "http",
		Host:   config.GraderAddress,
		Path:   "/list",
	}
	resp, err := http.Get(u.String())
	if err != nil {
		log.Fatalf("Failed to load problem type list from %s: %v", u.String(), err)
	}
	if resp.StatusCode != 200 {
		log.Fatalf("Go response %d: %s from %s", resp.StatusCode, resp.Status, u.String())
	}
	defer resp.Body.Close()

	var list []*ProblemType
	if err = json.NewDecoder(resp.Body).Decode(&list); err != nil {
		log.Fatalf("Failed to decode response from %s: %v", u.String(), err)
	}

	if len(list) == 0 {
		log.Fatalf("List of problem types from %s is empty", u.String())
	}

	if i := db.Del("grader:problemtypes"); i.Err() != nil {
		log.Fatalf("DB error deleting problem type hash: %v", i.Err())
	}

	for _, elt := range list {
		log.Printf("Adding %s problem type", elt.Tag)
		raw, err := json.Marshal(elt)
		if err != nil {
			log.Fatalf("Error re-encoding problem type description for %s: %v", elt.Tag, err)
		}
		if b := db.HSet("grader:problemtypes", elt.Tag, string(raw)); b.Err() != nil {
			log.Fatalf("DB error adding %s problem type: %v", elt.Tag, b.Err())
		}
	}
}

func problem_types(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	// get the list of types from the database
	slice := db.HGetAll("grader:problemtypes")
	if slice.Err() != nil {
		log.Printf("DB error getting list of problem types: %v", slice.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	lst := slice.Val()
	types := make(map[string]*ProblemType)
	for i := 0; i < len(lst); i += 2 {
		tag, raw := lst[i], lst[i+1]

		// parse JSON data
		problemType := new(ProblemType)
		if err := json.Unmarshal([]byte(raw), problemType); err != nil {
			log.Printf("Unable to parse JSON type description: %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}

		types[tag] = problemType
	}

	writeJson(w, r, types)
}

func problem_type(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	tag := r.URL.Query().Get(":tag")

	// get the field list in JSON form
	str := db.HGet("grader:problemtypes", tag)
	if str.Err() != nil {
		log.Printf("DB error getting type description for %s: %v", tag, str.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	s := str.Val()
	if len(s) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// parse JSON data
	problemType := new(ProblemType)
	if err := json.Unmarshal([]byte(s), problemType); err != nil {
		log.Printf("Unable to parse JSON response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	writeJson(w, r, problemType)
}

type Problem struct {
	ID   int64
	Name string
	Type string
	Tags []string
	Data map[string]interface{}
}

func problem_new(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session, decoder *json.Decoder) {
	problem_save_common(w, r, db, session, decoder, 0)
}

func problem_update(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session, decoder *json.Decoder) {
	id := r.URL.Query().Get(":id")
	if id == "" {
		log.Printf("Missing ID")
		http.Error(w, "Missing ID", http.StatusBadRequest)
		return
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		log.Printf("Error parsing ID [%s]: %v", id, err)
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if n < 1 {
		log.Printf("Invalid ID < 0 [%s]", id)
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	problem_save_common(w, r, db, session, decoder, n)
}

func problem_save_common(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session, decoder *json.Decoder, id int64) {
	instructor := session.Values["email"].(string)

	problem := new(Problem)
	if err := decoder.Decode(problem); err != nil {
		log.Printf("Failure decoding JSON request: %v", err)
		http.Error(w, "Failure decoding JSON request", http.StatusBadRequest)
		return
	}

	problem.ID = id

	// make sure it has a name
	problem.Name = strings.TrimSpace(problem.Name)
	if problem.Name == "" {
		log.Printf("Problem missing name")
		http.Error(w, "Problem missing name", http.StatusBadRequest)
		return
	}

	// must have at least one valid tag
	if len(problem.Tags) == 0 {
		log.Printf("Problem missing tags")
		http.Error(w, "Problem missing tags", http.StatusBadRequest)
		return
	}
	for _, elt := range problem.Tags {
		if !validProblemTag(elt) {
			log.Printf("Problem has invalid tag: %s", elt)
			http.Error(w, "Problem has invalid tag", http.StatusBadRequest)
			return
		}
	}

	// must be a recognized problem type
	str := db.HGet("grader:problemtypes", problem.Type)
	if str.Err() != nil {
		log.Printf("DB error getting type description for %s: %v", problem.Type, str.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	s := str.Val()
	if len(s) == 0 {
		log.Printf("Problem has unrecognized type: %s", problem.Type)
		http.Error(w, "Unknown problem type", http.StatusBadRequest)
		return
	}

	// parse JSON data describing problem type
	problemType := new(ProblemType)
	if err := json.Unmarshal([]byte(s), problemType); err != nil {
		log.Printf("Unable to parse JSON response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// validate the problem and prepare for storage
	filtered := make(map[string]interface{})

	for _, field := range problemType.FieldList {
		if value, present := problem.Data[field.Name]; present && field.Creator == "edit" {
			filtered[field.Name] = value
		} else if field.Creator == "edit" {
			log.Printf("Missing %s field in problem", field.Name)
			http.Error(w, "Problem data missing required field", http.StatusBadRequest)
			return
		}
	}
	problem.Data = filtered

	problemJson, err := json.Marshal(problem)
	if err != nil {
		log.Printf("JSON encoding error: %v", err)
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		return
	}

	// store the new problem in the database
	iface := db.EvalSha(luaScripts["saveproblem"], []string{}, []string{
		instructor,
		string(problemJson),
	})
	if iface.Err() != nil {
		log.Printf("DB error saving problem: %v", iface.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	newproblem := iface.Val().(string)

	// decode the problem object
	final := new(Problem)
	if err := json.Unmarshal([]byte(newproblem), final); err != nil {
		log.Printf("Unable to parse JSON response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	writeJson(w, r, final)
}

func problem_get(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	id := r.URL.Query().Get(":id")

	b := db.SIsMember("index:problems:all", id)
	if b.Err() != nil {
		log.Printf("DB error checking if problem exists: %v", b.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if !b.Val() {
		log.Printf("Problem [%s] not found", id)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	iface := db.EvalSha(luaScripts["problemget"], []string{}, []string{id})
	if iface.Err() != nil {
		log.Printf("DB error getting problem: %v", iface.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	raw := iface.Val().(string)

	// decode the problem object
	problem := new(Problem)
	if err := json.Unmarshal([]byte(raw), problem); err != nil {
		log.Printf("Unable to parse JSON response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	writeJson(w, r, problem)
}

func problem_tags(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	iface := db.EvalSha(luaScripts["problemtags"], nil, []string{})
	if iface.Err() != nil {
		log.Printf("DB error getting tags listing: %v", iface.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	var response interface{}
	if err := json.Unmarshal([]byte(iface.Val().(string)), &response); err != nil {
		log.Printf("Unable to parse JSON response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	writeJson(w, r, response)
}
