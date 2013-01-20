package main

import (
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
)

func init() {
	r := pat.New()
	r.Add("GET", `/problem/listtypes`, handlerStudent(problem_listtypes))
	r.Add("GET", `/problem/type/{tag:[\w:]+$}`, handlerStudent(problem_type))
	http.Handle("/problem/", r)
}

type ProblemDescriptionField struct {
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
	Name        string
	Tag         string
	Description []ProblemDescriptionField
}

var Python27InputOutputDescription = &ProblemType{
	Name: "Python 2.7 Input/Output",
	Tag:  "python27inputoutput",
	Description: []ProblemDescriptionField{
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
	Description: []ProblemDescriptionField{
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

func problem_listtypes(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	writeJson(w, r, []*ProblemType{Python27InputOutputDescription, Python27ExpressionDescription})
}

func problem_type(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	log.Printf("Not implemented")
}
