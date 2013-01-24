package main

import (
	"encoding/json"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
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
	Editor  string
	Student string
}

type ProblemType struct {
	Name      string
	Tag       string
	FieldList []ProblemField
}

var Python27InputOutputDescription = &ProblemType{
	Name: "Python 2.7 Input/Output",
	Tag:  "python27inputoutput",
	FieldList: []ProblemField{
		{
			Name:    "Description",
			Prompt:  "Enter the problem description here",
			Title:   "Problem description",
			Type:    "markdown",
			Editor:  "edit",
			Student: "view",
		},
		{
			Name:    "Reference",
			Prompt:  "Enter the reference solution here",
			Title:   "Reference solution",
			Type:    "python",
			Editor:  "edit",
			Student: "nothing",
		},
		{
			Name:    "TestCases",
			Prompt:  "Test cases",
			Title:   "Test cases",
			Type:    "text",
			List:    true,
			Editor:  "edit",
			Student: "view",
		},
		{
			Name:    "Candidate",
			Prompt:  "Enter your solution here",
			Title:   "Student solution",
			Type:    "python",
			Editor:  "nothing",
			Student: "edit",
		},
		{
			Name:    "Seconds",
			Prompt:  "Max time permitted in seconds",
			Title:   "Max time permitted in seconds",
			Type:    "int",
			Default: "10",
			Editor:  "edit",
			Student: "view",
		},
		{
			Name:    "MB",
			Prompt:  "Max memory permitted in megabytes",
			Title:   "Max memory permitted in megabytes",
			Type:    "int",
			Default: "32",
			Editor:  "edit",
			Student: "view",
		},
	},
}

var Python27ExpressionDescription = &ProblemType{
	Name: "Python 2.7 Expression",
	Tag:  "python27expression",
	FieldList: []ProblemField{
		{
			Name:    "Description",
			Prompt:  "Enter the problem description here",
			Title:   "Problem description",
			Type:    "markdown",
			Editor:  "edit",
			Student: "view",
		},
		{
			Name:    "Reference",
			Prompt:  "Enter the reference solution here",
			Title:   "Reference solution",
			Type:    "python",
			Editor:  "edit",
			Student: "nothing",
		},
		{
			Name:    "TestCases",
			Prompt:  "Test cases",
			Title:   "Test cases",
			Type:    "string",
			List:    true,
			Editor:  "edit",
			Student: "view",
		},
		{
			Name:    "Candidate",
			Prompt:  "Enter your solution here",
			Title:   "Student solution",
			Type:    "python",
			Editor:  "nothing",
			Student: "edit",
		},
		{
			Name:    "Seconds",
			Prompt:  "Max time permitted in seconds",
			Title:   "Max time permitted in seconds",
			Type:    "int",
			Default: "10",
			Editor:  "edit",
			Student: "view",
		},
		{
			Name:    "MB",
			Prompt:  "Max memory permitted in megabytes",
			Title:   "Max memory permitted in megabytes",
			Type:    "int",
			Default: "32",
			Editor:  "edit",
			Student: "view",
		},
	},
}

func setupProblemTypes(db *redis.Client) {
	ioJson, err := json.Marshal(Python27InputOutputDescription)
	if err != nil {
		log.Fatalf("Error json-encoding Python27InputOutputDescription: %v", err)
	}
	expJson, err := json.Marshal(Python27ExpressionDescription)
	if err != nil {
		log.Fatalf("Error json-encoding Python27ExpressionDescription: %v", err)
	}

	if i := db.Del("grader:problemtypes"); i.Err() != nil {
		log.Fatalf("DB error deleting problem type hash: %v", i.Err())
	}

	if b := db.HSet("grader:problemtypes", Python27InputOutputDescription.Tag, string(ioJson)); b.Err() != nil {
		log.Fatalf("DB error adding Python27InputOutputDescription type: %v", b.Err())
	}
	if b := db.HSet("grader:problemtypes", Python27ExpressionDescription.Tag, string(expJson)); b.Err() != nil {
		log.Fatalf("DB error adding Python27ExpressionDescription type: %v", b.Err())
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
	types := make(map[string]string)
	for i := 0; i < len(lst); i += 2 {
		types[lst[i]] = lst[i+1]
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

	problem.ID = 0

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
	if err := filterEditorFields(problemType, problem.Data); err != nil {
		log.Printf("Error validating problem: %v", err)
		http.Error(w, "Error validating problem", http.StatusBadRequest)
		return
	}
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
		log.Printf("DB error saving problem: %v", err)
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

func filterEditorFields(problemType *ProblemType, p map[string]interface{}) (filtered map[string]interface{}) {
	filtered = make(map[string]interface{})

	for _, field := range problemType.FieldList {
		if value, present := p[field.Name]; present && field.Editor == "edit" {
			filtered[field.Name] = value
		}
	}

	return
}
