package main

import (
	"encoding/csv"
	"fmt"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
)

func init() {
	r := pat.New()
	r.Add("POST", `/instructor/upload/courselist`, handlerInstructor(instructor_upload_courselist))
	http.Handle("/instructor/", r)
}

type CSVStudent struct {
	Name  string
	ID    string
	Email string
}

type CSVUploadResult struct {
	Success                bool
	Message                string
	UnknownCanvasCourseTag string
	UnknownStudents        []string
	Log                    []string
}

func instructor_upload_courselist(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	instructor := session.Values["email"].(string)
	role := session.Values["email"].(string)

	result := &CSVUploadResult{
		Success:         true,
		UnknownStudents: []string{},
		Log:             []string{},
	}

	// parse the csv file
	file, _, err := r.FormFile("CSVFile")
	if err != nil {
		log.Printf("instructor_upload_courselist called with no CSV file")
		http.Error(w, "No CSV file submitted", http.StatusBadRequest)
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrailingComma = true
	records, err := reader.ReadAll()
	if err != nil {
		log.Printf("Error parsing CSV file: %v", err)
		http.Error(w, "Error parsing CSV file", http.StatusBadRequest)
		return
	}

	// make sure there was at least one student after skipping headers
	if len(records) < 3 {
		log.Printf("File does not seem to contain any students")
		return
	}

	// throw away the header lines
	records = records[2:]

	// scan the list to see if we recognize the course and all the students
	course := ""
	for _, student := range records {
		if len(student) < 3 {
			log.Printf("Student record with too few fields: %v", student)
			http.Error(w, "Student record with too few fields", http.StatusBadRequest)
			return
		}
		name, id, section := student[0], student[1], student[2]

		// make sure this is a known course
		b := db.HExists("index:courses:tagbycanvastag", section)
		if b.Err() != nil {
			log.Printf("DB error checking for course tag for %s: %v", section, err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		tag := ""
		if b.Val() {
			// get the course tag
			str := db.HGet("index:course:tagbycanvastag", section)
			if str.Err() != nil {
				log.Printf("DB error getting course tag for %s: %v", section, err)
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
			tag = str.Val()

			// check if the course is active
			if b = db.SIsMember("index:courses:active", tag); b.Err() != nil {
				log.Printf("DB error checking if course is active for %s: %v", tag, err)
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
			if !b.Val() {
				log.Printf("Course is not active: %s", tag)
				http.Error(w, "Course is not active", http.StatusBadRequest)
				return
			}
		} else {
			// request the Canvas section -> tag name be filled in
			result.Success = false
			result.UnknownCanvasCourseTag = section
		}

		if course == "" && tag != "" {
			course = tag
		} else if course != "" && tag != "" && tag != course {
			log.Printf("Error: two courses found, %s and %s", course, tag)
			http.Error(w, "File contains data for more than one course", http.StatusBadRequest)
			return
		}

		// see if we know the email address for this student
		if b = db.HExists("index:students:emailbyid", id); b.Err() != nil {
			log.Printf("DB error checking for student ID %s for student %s: %v", id, name, err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		if !b.Val() {
			result.Success = false
			result.UnknownStudents = append(result.UnknownStudents, fmt.Sprintf("%s (%s)", name, id))
		}
	}

	// is this instructor over this course?
	if course != "" && role != "admin" {
		b := db.SIsMember("course:"+course+":instructors", instructor)
		if b.Err() != nil {
			log.Printf("DB error checking if %s is an instructor for %s: %v", instructor, course, err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}

		if !b.Val() {
			log.Printf("%s is not an instructor for %s", instructor, course)
			http.Error(w, "You are not an instructor for this course", http.StatusUnauthorized)
			return
		}
	}

	// do we need more info before continuing?
	if !result.Success {
		writeJson(w, r, result)
		return
	}

	if course == "" {
		log.Printf("Error: no course found")
		http.Error(w, "No course found", http.StatusBadRequest)
		return
	}

	// now loop through again and set the students
	emailToName := make(map[string]string)
	for _, student := range records {
		name, id := student[0], student[1]

		// get the email address
		str := db.HGet("index:students:emailbyid", id)
		if str.Err() != nil {
			log.Printf("DB error getting student email address for %s (%s): %v", name, id, err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		email := str.Val()

		// note the students we have found
		emailToName[email] = name

		// add this student to the course
		iface := db.EvalSha(luaScripts["addstudenttocourse"], []string{}, []string{email, name, course})
		if iface.Err() != nil {
			log.Printf("DB error adding student to course: %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}

		if iface.Val().(string) == "noop" {
			result.Log = append(result.Log, fmt.Sprintf("Student %s (%s) already in course", name, email))
		} else {
			result.Log = append(result.Log, fmt.Sprintf("Student %s (%s) added to %s", name, email, course))
		}
	}

	// check for students that were not in the list
	slice := db.SMembers("course:" + course + ":students")
	if slice.Err() != nil {
		log.Printf("DB error getting list of students in course %s: %v", course, err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	roll := slice.Val()
	for _, elt := range roll {
		if _, present := emailToName[elt]; !present {
			result.Log = append(result.Log, fmt.Sprintf("Student %s is in %s but not in this CSV file", elt, course))
		}
	}

	writeJson(w, r, result)
}
