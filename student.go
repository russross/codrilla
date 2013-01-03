package main

import (
	"fmt"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"
)

func init() {
	r := pat.New()
	r.Add("GET", `/student/list/courses`, handlerStudent(student_list_courses))
	http.Handle("/student/", r)
}

type AssignmentListing struct {
	Course     string
	Name       string
	Open       time.Time
	Close      time.Time
	Attempts   int64
	ToBeGraded int64
	Passed     bool
}

func getAssignmentListingGeneric(db *redis.Client, course, assignment string) (*AssignmentListing, error) {
	result := new(AssignmentListing)

	// get the problem id
	str := db.Get("assignment:" + assignment + ":problem")
	if str.Err() != nil {
		log.Printf("DB error getting problem ID for assignement %s: %v", assignment, str.Err())
		return nil, str.Err()
	}
	problem := str.Val()
	if problem == "" {
		log.Printf("assignment %s mapped to blank problem ID", assignment)
		return nil, fmt.Errorf("DB error")
	}

	// get the problem name
	if str = db.Get("problem:" + problem + ":name"); str.Err() != nil {
		log.Printf("DB error getting problem name: %v", str.Err())
		return nil, str.Err()
	}
	result.Name = str.Val()

	// get the open time
	if str = db.Get("assignment:" + assignment + ":open"); str.Err() != nil {
		log.Printf("DB error getting assignment open time: %v", str.Err())
		return nil, str.Err()
	}
	openInt, err := strconv.ParseInt(str.Val(), 10, 64)
	if err != nil {
		log.Printf("Error parsing assignment open date for %s: %v", assignment, err)
		return nil, err
	}
	result.Open = time.Unix(openInt, 0).In(timeZone)

	// get the close time
	if str = db.Get("assignment:" + assignment + ":close"); str.Err() != nil {
		log.Printf("DB error getting assignment close time: %v", str.Err())
		return nil, str.Err()
	}
	closeInt, err := strconv.ParseInt(str.Val(), 10, 64)
	if err != nil {
		log.Printf("Error parsing assignment close date for %s: %v", assignment, err)
		return nil, err
	}
	result.Close = time.Unix(closeInt, 0).In(timeZone)

	return result, nil
}

func getAssignmentListingStudent(db *redis.Client, course, assignment, email string, result *AssignmentListing) error {
	// get the student solution id
	str := db.HGet("student:"+email+":solutions:"+course, assignment)
	if str.Err() != nil {
		log.Printf("DB error getting solution ID: %v", str.Err())
		return str.Err()
	}
	solution := str.Val()

	// no solution yet?
	if solution == "" {
		return nil
	}

	// get the number of attempts
	n := db.LLen("solution:" + solution + ":submissions")
	if n.Err() != nil {
		log.Printf("DB error getting submission count: %v", n.Err())
		return n.Err()
	}
	result.Attempts = n.Val()

	// get the number of graded solutions
	if n = db.LLen("solution:" + solution + ":submissions"); n.Err() != nil {
		log.Printf("DB error getting graded count: %v", n.Err())
		return n.Err()
	}
	result.ToBeGraded = result.Attempts - n.Val()

	// see if the student has passed yet
	if str = db.HGet("student:"+email+":solutions:"+course, assignment); str.Err() != nil {
		log.Printf("DB error getting passed field: %v", str.Err())
		return str.Err()
	}
	result.Passed = str.Val() == "true"

	return nil
}

type CourseListing struct {
	Name            string
	Close           time.Time
	Instructors     []string
	OpenAssignments []*AssignmentListing
	NextAssignment  *AssignmentListing
}

func getCourseListing(db *redis.Client, course string) (*CourseListing, error) {
	result := &CourseListing{
		Instructors:     []string{},
		OpenAssignments: []*AssignmentListing{},
	}

	// get the course name
	str := db.Get("course:" + course + ":name")
	if str.Err() != nil {
		log.Printf("DB error getting course name: %v", str.Err())
		return nil, str.Err()
	}
	result.Name = str.Val()

	// get the course closing time
	if str = db.Get("course:" + course + ":close"); str.Err() != nil {
		log.Printf("DB error getting course close: %v", str.Err())
		return nil, str.Err()
	}
	closeInt, err := strconv.ParseInt(str.Val(), 10, 64)
	if err != nil {
		log.Printf("Error parsing course close date for %s: %v", course, err)
		return nil, err
	}
	result.Close = time.Unix(closeInt, 0).In(timeZone)

	// get the course instructors
	slice := db.SMembers("course:" + course + ":instructors")
	if slice.Err() != nil {
		log.Printf("DB error getting course instructors: %v", slice.Err())
		return nil, slice.Err()
	}
	instructors := slice.Val()
	sort.Strings(instructors)
	result.Instructors = instructors

	return result, nil
}

type ListCoursesResponse struct {
	Email     string
	Name      string
	TimeStamp time.Time
	Courses   []*CourseListing
}

// get a list of current courses and assignments for this student
func student_list_courses(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	now := time.Now().In(timeZone)
	email := session.Values["email"].(string)
	email = "smoore6@dmail.dixie.edu"

	// build the response object
	response := &ListCoursesResponse{
		Email:     email,
		TimeStamp: now,
		Courses:   []*CourseListing{},
	}

	// get the user's name
	str := db.Get("student:" + email + ":name")
	if str.Err() != nil {
		log.Printf("DB error getting student name: %v", str.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	response.Name = str.Val()

	// get the list of courses
	slice := db.SMembers("student:" + email + ":courses")
	if slice.Err() != nil {
		log.Printf("DB error getting student course list for %s: %v", email, slice.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// get data for each course
	courseList := slice.Val()
	sort.Strings(courseList)
	for _, courseTag := range courseList {
		// get the generic course data
		course, err := getCourseListing(db, courseTag)
		if err != nil {
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}

		// get the list of active assignments
		if slice = db.SMembers("course:" + courseTag + ":assignments:active"); slice.Err() != nil {
			log.Printf("DB error getting assignment list for course %s: %v", courseTag, slice.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		assignments := slice.Val()
		for _, asstID := range assignments {
			assignment, err := getAssignmentListingGeneric(db, courseTag, asstID)
			if err != nil {
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
			if err = getAssignmentListingStudent(db, courseTag, asstID, email, assignment); err != nil {
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
			course.OpenAssignments = append(course.OpenAssignments, assignment)
		}

		// get the next assignment that will be posted
		if slice = db.ZRange("course:"+courseTag+":assignments:futurebyopen", 0, 1); slice.Err() != nil {
			log.Printf("DB error getting next assignment for course %s: %v", courseTag, slice.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		future := slice.Val()
		if len(future) == 0 {
			course.NextAssignment = nil
		} else {
			course.NextAssignment, err = getAssignmentListingGeneric(db, courseTag, future[0])
			if err != nil {
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
		}

		response.Courses = append(response.Courses, course)
	}

	writeJson(w, r, response)
}
