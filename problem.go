package main

import (
	"database/sql"
	"encoding/json"
	"github.com/gorilla/pat"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var problemTypes map[string]*ProblemType

func init() {
	r := pat.New()
	r.Add("GET", `/problem/types`, handlerInstructor(problem_types))
	r.Add("GET", `/problem/type/{tag:[\w:]+$}`, handlerInstructor(problem_type))
	r.Add("GET", `/problem/get/{id:\d+$}`, handlerInstructor(problem_get))
	r.Add("GET", `/problem/tags`, handlerInstructor(problem_tags))
	r.Add("POST", `/problem/new`, handlerInstructorJson(problem_new))
	r.Add("POST", `/problem/update/{id:\d+$}`, handlerInstructorJson(problem_update))
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

func setupProblemTypes() {
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

	problemTypes = make(map[string]*ProblemType)

	for _, elt := range list {
		log.Printf("Adding %s problem type", elt.Tag)
		problemTypes[elt.Tag] = elt
	}
}

type ProblemTypesResponseElt struct {
	Name string
	Tag  string
}

func problem_types(w http.ResponseWriter, r *http.Request, instructor *InstructorDB) {
	list := []*ProblemTypesResponseElt{}
	for _, elt := range problemTypes {
		list = append(list, &ProblemTypesResponseElt{Name: elt.Name, Tag: elt.Tag})
	}
	writeJson(w, r, list)
}

func problem_type(w http.ResponseWriter, r *http.Request, instructor *InstructorDB) {
	tag := r.URL.Query().Get(":tag")

	problemType, present := problemTypes[tag]
	if !present {
		log.Printf("Problem type %s not found", tag)
		http.Error(w, "Problem type not found", http.StatusNotFound)
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

func problem_new(w http.ResponseWriter, r *http.Request, db *sql.DB, instructor *InstructorDB, decoder *json.Decoder) {
	problem_save_common(w, r, db, instructor, decoder, -1)
}

func problem_update(w http.ResponseWriter, r *http.Request, db *sql.DB, instructor *InstructorDB, decoder *json.Decoder) {
	id, err := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)
	if err != nil {
		log.Printf("Bad ID %s: %v", r.URL.Query().Get(":id"), err)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if id < 0 {
		log.Printf("Invalid ID: %d", id)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	problem_save_common(w, r, db, instructor, decoder, id)
}

func problem_save_common(w http.ResponseWriter, r *http.Request, db *sql.DB, instructor *InstructorDB, decoder *json.Decoder, id int64) {
	problem := new(Problem)
	if err := decoder.Decode(problem); err != nil {
		log.Printf("Failure decoding JSON request: %v", err)
		http.Error(w, "Failure decoding JSON request", http.StatusBadRequest)
		return
	}

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
	problemType, present := problemTypes[problem.Type]
	if !present {
		log.Printf("Problem has unrecognized type: %s", problem.Type)
		http.Error(w, "Unknown problem type", http.StatusBadRequest)
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

	problemJson, err := json.Marshal(problem.Data)
	if err != nil {
		log.Printf("JSON encoding error: %v", err)
		http.Error(w, "JSON encoding error", http.StatusInternalServerError)
		return
	}

	// make sure the problem exists (if an update)
	if id >= 0 {
		if _, present := problemsByID[id]; !present {
			log.Printf("Problem %d does not exist", id)
			http.Error(w, "Problem not found", http.StatusNotFound)
			return
		}
	}

	// store the new problem in the database
	txn, err := db.Begin()
	if err != nil {
		log.Printf("DB error starting transaction: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer txn.Rollback()

	if id >= 0 {
		// update in place
		_, err := txn.Exec("update Problem set Name = ?, Type = ?, Data = ? where ID = ?",
			problem.Name,
			problem.Type,
			problemJson,
			id)
		if err != nil {
			log.Printf("DB error updating Problem %d: %v", id, err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		problem.ID = id
	} else {
		// create new
		result, err := txn.Exec("insert into Problem values (null, ?, ?, ?)",
			problem.Name,
			problem.Type,
			problemJson)
		if err != nil {
			log.Printf("DB error inserting Problem: %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		newid, err := result.LastInsertId()
		if err != nil {
			log.Printf("DB error getting ID of newly inserted Problem: %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		problem.ID = newid
	}

	// update tags
	if id >= 0 {
		// delete old tags
		_, err := txn.Exec("delete from ProblemTag where Problem = ?", id)
		if err != nil {
			log.Printf("DB error clearing old tags for problem %d: %v", id, err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
	}

	// insert tag links
	for _, tag := range problem.Tags {
		if _, present := tagsByTag[tag]; !present {
			_, err := txn.Exec("insert into Tag values (?, ?, ?)", tag, tag, 0)
			if err != nil {
				log.Printf("DB error inserting Tag %s: %v", tag, err)
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
		}
		_, err := txn.Exec("insert into ProblemTag values (?, ?)", problem.ID, tag)
		if err != nil {
			log.Printf("DB error inserting ProblemTag problem %d tag %s: %v", problem.ID, tag, err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
	}

	if err = txn.Commit(); err != nil {
		log.Printf("DB error committing: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// update in-memory version
	var p *ProblemDB
	if id >= 0 {
		// update in place
		p = problemsByID[id]
		p.Name = problem.Name
		p.Type = problemType
		p.Data = problem.Data

		// delete old tag links
		for _, tag := range p.Tags {
			delete(p.Tags, tag.Tag)
			delete(tag.Problems, id)
		}
	} else {
		// create new
		p = &ProblemDB{
			ID:          problem.ID,
			Name:        problem.Name,
			Type:        problemType,
			Data:        problem.Data,
			Tags:        make(map[string]*TagDB),
			Assignments: make(map[int64]*AssignmentDB),
			Courses:     make(map[string]*CourseDB),
		}
		problemsByID[problem.ID] = p
	}

	// create tag links
	for _, tagName := range problem.Tags {
		tag, present := tagsByTag[tagName]
		if !present {
			// create a new tag
			tag = &TagDB{
				Tag:         tagName,
				Description: tagName,
				Priority:    0,
				Problems:    make(map[int64]*ProblemDB),
			}
			tagsByTag[tagName] = tag
		}
		tag.Problems[problem.ID] = p
		p.Tags[tagName] = tag
	}

	// return the final problem, complete with new ID (if applicable)
	final := getProblem(p)

	writeJson(w, r, final)
}

type ProblemGetResponse struct {
	ID   int64
	Name string
	Type string
	Tags []string
	Data map[string]interface{}
}

func getProblem(problem *ProblemDB) *ProblemGetResponse {
	// assemble and sort list of tags
	tags := []string{}
	for tag, _ := range problem.Tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	resp := &ProblemGetResponse{
		ID:   problem.ID,
		Name: problem.Name,
		Type: problem.Type.Tag,
		Tags: tags,
		Data: problem.Data,
	}

	return resp
}

func problem_get(w http.ResponseWriter, r *http.Request, instructor *InstructorDB) {
	id, err := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)
	if err != nil {
		log.Printf("Parse error for ID %s: %v", r.URL.Query().Get(":id"), err)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// look up the problem
	problem, present := problemsByID[id]
	if !present {
		log.Printf("Problem %d not found", id)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	resp := getProblem(problem)

	writeJson(w, r, resp)
}

type ProblemTagsResponse struct {
	Tags     []*TagListing
	Problems []*ProblemListing
}

type TagListing struct {
	Tag         string
	Description string
	Priority    int64
	Problems    []int64
}

type ProblemListing struct {
	ID     int64
	Name   string
	Type   string
	Tags   []string
	UsedBy []string
}

type TagsByPriority []*TagListing

func (p TagsByPriority) Len() int           { return len(p) }
func (p TagsByPriority) Less(i, j int) bool { return p[i].Priority < p[j].Priority }
func (p TagsByPriority) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type Int64Slice []int64

func (p Int64Slice) Len() int           { return len(p) }
func (p Int64Slice) Less(i, j int) bool { return p[i] < p[j] }
func (p Int64Slice) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func problem_tags(w http.ResponseWriter, r *http.Request, instructor *InstructorDB) {
	resp := &ProblemTagsResponse{
		Tags:     []*TagListing{},
		Problems: []*ProblemListing{},
	}

	// gather tags
	for _, tag := range tagsByTag {
		problems := []int64{}
		for id, _ := range tag.Problems {
			problems = append(problems, id)
		}
		sort.Sort(Int64Slice(problems))
		elt := &TagListing{
			Tag:         tag.Tag,
			Description: tag.Description,
			Priority:    tag.Priority,
			Problems:    problems,
		}
		resp.Tags = append(resp.Tags, elt)
	}
	sort.Sort(TagsByPriority(resp.Tags))

	// gather problems
	for _, problem := range problemsByID {
		tags := []string{}
		for tag, _ := range problem.Tags {
			tags = append(tags, tag)
		}
		sort.Strings(tags)
		usedby := []string{}
		for course, _ := range problem.Courses {
			usedby = append(usedby, course)
		}
		sort.Strings(usedby)
		elt := &ProblemListing{
			ID:     problem.ID,
			Name:   problem.Name,
			Type:   problem.Type.Tag,
			Tags:   tags,
			UsedBy: usedby,
		}
		resp.Problems = append(resp.Problems, elt)
	}

	writeJson(w, r, resp)
}
