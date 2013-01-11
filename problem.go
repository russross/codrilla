package main

import (
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"net/http"
)

func init() {
	r := pat.New()
	r.Add("GET", `/problem/listtypes`, handlerStudent(problem_listtypes))
	http.Handle("/problem/", r)
}

type ProblemDescriptionField struct {
	Name       string
	Prompt     string
	Title      string
	Type       string
	Default    string
	Instructor string
	Student    string
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
			Name:       "Description",
			Prompt:     "Enter the problem description here",
			Title:      "Problem description",
			Type:       "markdown",
			Instructor: "edit",
			Student:    "view",
		},
		{
			Name:       "Reference",
			Prompt:     "Enter the reference solution here",
			Title:      "Reference solution",
			Type:       "python",
			Instructor: "edit",
			Student:    "nothing",
		},
		{
			Name:       "TestCases",
			Prompt:     "Test cases",
			Title:      "Test cases",
			Type:       "textfilelist",
			Instructor: "edit",
			Student:    "view",
		},
		{
			Name:       "Candidate",
			Prompt:     "Enter your solution here",
			Title:      "Student solution",
			Type:       "python",
			Instructor: "nothing",
			Student:    "edit",
		},
		{
			Name:       "Seconds",
			Prompt:     "Max time permitted in seconds",
			Title:      "Max time permitted in seconds",
			Type:       "int",
			Default:    "10",
			Instructor: "edit",
			Student:    "view",
		},
		{
			Name:       "MB",
			Prompt:     "Max memory permitted in megabytes",
			Title:      "Max memory permitted in megabytes",
			Type:       "int",
			Default:    "32",
			Instructor: "edit",
			Student:    "view",
		},
	},
}

var Python27ExpressionDescription = &ProblemType{
	Name: "Python 2.7 Expression",
	Tag:  "python27expression",
	Description: []ProblemDescriptionField{
		{
			Name:       "Description",
			Prompt:     "Enter the problem description here",
			Title:      "Problem description",
			Type:       "markdown",
			Instructor: "edit",
			Student:    "view",
		},
		{
			Name:       "Reference",
			Prompt:     "Enter the reference solution here",
			Title:      "Reference solution",
			Type:       "python",
			Instructor: "edit",
			Student:    "nothing",
		},
		{
			Name:       "TestCases",
			Prompt:     "Test cases",
			Title:      "Test cases",
			Type:       "textfieldlist",
			Instructor: "edit",
			Student:    "view",
		},
		{
			Name:       "Candidate",
			Prompt:     "Enter your solution here",
			Title:      "Student solution",
			Type:       "python",
			Instructor: "nothing",
			Student:    "edit",
		},
		{
			Name:       "Seconds",
			Prompt:     "Max time permitted in seconds",
			Title:      "Max time permitted in seconds",
			Type:       "int",
			Default:    "10",
			Instructor: "edit",
			Student:    "view",
		},
		{
			Name:       "MB",
			Prompt:     "Max memory permitted in megabytes",
			Title:      "Max memory permitted in megabytes",
			Type:       "int",
			Default:    "32",
			Instructor: "edit",
			Student:    "view",
		},
	},
}

func problem_listtypes(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	writeJson(w, r, []*ProblemType{ Python27InputOutputDescription, Python27ExpressionDescription })
}
