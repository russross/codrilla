package main

import (
	"database/sql"
	"encoding/json"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"sync"
	"time"
)

var database *sql.DB
var mutex sync.RWMutex

func initDatabase() {
	mutex.Lock()
	db, err := sql.Open("sqlite3", config.DatabaseName)
	if err != nil {
		log.Fatalf("Error opening %s: %v", config.DatabaseName, err)
	}

	// read entire database into memory, one table at a time
	log.Printf("reading %s", config.DatabaseName)
	ScanAdministratorTable(db)
	ScanInstructorTable(db)
	ScanStudentTable(db)
	ScanCourseTable(db)
	ScanCourseInstructorTable(db)
	ScanCourseStudentTable(db)
	ScanTagTable(db)
	ScanProblemTable(db)
	ScanProblemTagTable(db)
	ScanAssignmentTable(db)
	ScanSolutionTable(db)
	ScanSubmissionTable(db)

	database = db
	mutex.Unlock()
}

//
// Data types
//

// administratorsByEmail[email]
type AdministratorDB struct {
	Email string
	Name  string
}

var administratorsByEmail = make(map[string]*AdministratorDB)

func ScanAdministratorTable(db *sql.DB) {
	rows, err := db.Query("select * from Administrator")
	if err != nil {
		log.Fatalf("DB error selecting from Administrator: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(AdministratorDB)
		if err = rows.Scan(&elt.Email, &elt.Name); err != nil {
			log.Fatalf("DB error scanning Administrator: %v", err)
		}
		administratorsByEmail[elt.Email] = elt
	}
}

// instructorsByEmail[email]
// CourseDB.Instructors[email]
type InstructorDB struct {
	Email string
	Name  string

	Courses map[string]*CourseDB
}

var instructorsByEmail = make(map[string]*InstructorDB)

func ScanInstructorTable(db *sql.DB) {
	rows, err := db.Query("select * from Instructor")
	if err != nil {
		log.Fatalf("DB error selecting from Instructor: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(InstructorDB)
		elt.Courses = make(map[string]*CourseDB)
		if err = rows.Scan(&elt.Email, &elt.Name); err != nil {
			log.Fatalf("DB error scanning Instructor: %v", err)
		}
		instructorsByEmail[elt.Email] = elt
	}
}

// studentsByEmail[email]
// CourseDB.Students[email]
// AssignmentDB.SolutionsByStudent[email] = SolutionDB
// SolutionDB.Student
type StudentDB struct {
	Email                 string
	Name                  string
	Courses               map[string]*CourseDB
	SolutionsByAssignment map[int64]*SolutionDB
}

var studentsByEmail = make(map[string]*StudentDB)

func ScanStudentTable(db *sql.DB) {
	rows, err := db.Query("select * from Student")
	if err != nil {
		log.Fatalf("DB error selecting from Student: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(StudentDB)
		elt.Courses = make(map[string]*CourseDB)
		elt.SolutionsByAssignment = make(map[int64]*SolutionDB)
		if err = rows.Scan(&elt.Email, &elt.Name); err != nil {
			log.Fatalf("DB error scanning Student: %v", err)
		}
		studentsByEmail[elt.Email] = elt
	}
}

// coursesByTag[tag]
// InstructorDB.Courses[tag]
// StudentDB.Courses[tag]
// ProblemDB.Courses[tag]
// AssignmentDB.Course
type CourseDB struct {
	Tag   string
	Name  string
	Close time.Time

	Instructors map[string]*InstructorDB
	Students    map[string]*StudentDB
	Assignments map[int64]*AssignmentDB
}

var coursesByTag = make(map[string]*CourseDB)

func ScanCourseTable(db *sql.DB) {
	rows, err := db.Query("select * from Course")
	if err != nil {
		log.Fatalf("DB error selecting from Course: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(CourseDB)
		elt.Instructors = make(map[string]*InstructorDB)
		elt.Students = make(map[string]*StudentDB)
		elt.Assignments = make(map[int64]*AssignmentDB)
		if err = rows.Scan(&elt.Tag, &elt.Name, &elt.Close); err != nil {
			log.Fatalf("DB error scanning Course: %v", err)
		}
		coursesByTag[elt.Tag] = elt
	}
}

func ScanCourseInstructorTable(db *sql.DB) {
	rows, err := db.Query("select * from CourseInstructor")
	if err != nil {
		log.Fatalf("DB error selecting from CourseInstructor: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var course, instructor string
		if err = rows.Scan(&course, &instructor); err != nil {
			log.Fatalf("DB error scanning CourseInstructor: %v", err)
		}
		coursesByTag[course].Instructors[instructor] = instructorsByEmail[instructor]
		instructorsByEmail[instructor].Courses[course] = coursesByTag[course]
	}
}

func ScanCourseStudentTable(db *sql.DB) {
	rows, err := db.Query("select * from CourseStudent")
	if err != nil {
		log.Fatalf("DB error selecting from CourseStudent: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var course, student string
		if err = rows.Scan(&course, &student); err != nil {
			log.Fatalf("DB error scanning CourseStudent: %v", err)
		}
		coursesByTag[course].Students[student] = studentsByEmail[student]
		studentsByEmail[student].Courses[course] = coursesByTag[course]
	}
}

// tagsByTag[tag]
// ProblemDB.Tags[tag]
type TagDB struct {
	Tag         string
	Description string
	Priority    int64
	Problems    map[int64]*ProblemDB
}

var tagsByTag = make(map[string]*TagDB)

func ScanTagTable(db *sql.DB) {
	rows, err := db.Query("select * from Tag")
	if err != nil {
		log.Fatalf("DB error selecting from Tag: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(TagDB)
		elt.Problems = make(map[int64]*ProblemDB)
		if err = rows.Scan(&elt.Tag, &elt.Description, &elt.Priority); err != nil {
			log.Fatalf("DB error scanning Tag: %v", err)
		}
		tagsByTag[elt.Tag] = elt
	}
}

// problemsByID[problemID]
// TagDB.Problems[problemID]
// AssignmentDB.Problem
type ProblemDB struct {
	ID          int64
	Name        string
	Type        *ProblemType
	Data        map[string]interface{}
	Tags        map[string]*TagDB
	Assignments map[int64]*AssignmentDB
	Courses     map[string]*CourseDB
}

var problemsByID = make(map[int64]*ProblemDB)
var outputByProblemID = make(map[int64]interface{})

func ScanProblemTable(db *sql.DB) {
	rows, err := db.Query("select * from Problem")
	if err != nil {
		log.Fatalf("DB error selecting from Problem: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(ProblemDB)
		elt.Tags = make(map[string]*TagDB)
		elt.Assignments = make(map[int64]*AssignmentDB)
		elt.Courses = make(map[string]*CourseDB)
		var typename string
		var dataJson string
		if err = rows.Scan(&elt.ID, &elt.Name, &typename, &dataJson); err != nil {
			log.Fatalf("DB error scanning Problem: %v", err)
		}
		problemType, present := problemTypes[typename]
		if !present {
			log.Fatalf("Problem %d found with unknown type %s", elt.ID, typename)
		}
		elt.Type = problemType
		if err = json.Unmarshal([]byte(dataJson), &elt.Data); err != nil {
			log.Fatalf("JSON error in Problem Data for Problem %d: %v", elt.ID, err)
		}
		problemsByID[elt.ID] = elt
	}
}

func ScanProblemTagTable(db *sql.DB) {
	rows, err := db.Query("select * from ProblemTag")
	if err != nil {
		log.Fatalf("DB error selecting from ProblemTag: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var problem int64
		var tag string
		if err = rows.Scan(&problem, &tag); err != nil {
			log.Fatalf("DB error scanning ProblemTag: %v", err)
		}
		problemsByID[problem].Tags[tag] = tagsByTag[tag]
		tagsByTag[tag].Problems[problem] = problemsByID[problem]
	}
}

// assignmentsByID[asstID]
// CourseDB.Assignments[asstID]
// ProblemDB.Assignments[asstID]
// SolutionDB.Assignment
type AssignmentDB struct {
	ID                 int64
	Course             *CourseDB
	Problem            *ProblemDB
	ForCredit          bool
	Open               time.Time
	Close              time.Time
	SolutionsByStudent map[string]*SolutionDB
}

var assignmentsByID = make(map[int64]*AssignmentDB)

func ScanAssignmentTable(db *sql.DB) {
	rows, err := db.Query("select * from Assignment")
	if err != nil {
		log.Fatalf("DB error selecting from Assignment: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(AssignmentDB)
		elt.SolutionsByStudent = make(map[string]*SolutionDB)
		var course string
		var problem int64
		if err = rows.Scan(&elt.ID, &course, &problem, &elt.ForCredit, &elt.Open, &elt.Close); err != nil {
			log.Fatalf("DB error scanning Assignment: %v", err)
		}
		elt.Course = coursesByTag[course]
		elt.Problem = problemsByID[problem]
		assignmentsByID[elt.ID] = elt
		elt.Problem.Assignments[elt.ID] = elt
		elt.Course.Assignments[elt.ID] = elt

		// let problems know which courses use them
		elt.Problem.Courses[course] = elt.Course
	}
}

// solutionsByID[id]
// StudentDB.SolutionsByAssignment[asstID]
// AssignmentDB.SolutionsByStudent[email]
// SubmissionDB.Solution
type SolutionDB struct {
	ID                 int64
	Student            *StudentDB
	Assignment         *AssignmentDB
	SubmissionsInOrder []*SubmissionDB
}

var solutionsByID = make(map[int64]*SolutionDB)

func ScanSolutionTable(db *sql.DB) {
	rows, err := db.Query("select * from Solution")
	if err != nil {
		log.Fatalf("DB error selecting from Solution: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(SolutionDB)
		var student string
		var assignment int64
		if err = rows.Scan(&elt.ID, &student, &assignment); err != nil {
			log.Fatalf("DB error scanning Solution: %v", err)
		}
		elt.Student = studentsByEmail[student]
		elt.Assignment = assignmentsByID[assignment]
		solutionsByID[elt.ID] = elt
		elt.Student.SolutionsByAssignment[assignment] = elt
		elt.Assignment.SolutionsByStudent[student] = elt
	}
}

// SolutionDB.SubmissionsInOrder[]
type SubmissionDB struct {
	Solution    *SolutionDB
	TimeStamp   time.Time
	Submission  map[string]interface{}
	GradeReport map[string]interface{}
	Passed      bool
}

func ScanSubmissionTable(db *sql.DB) {
	rows, err := db.Query("select * from Submission order by TimeStamp")
	if err != nil {
		log.Fatalf("DB error selecting from Submission: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		elt := new(SubmissionDB)
		var solution int64
		var submissionJson string
		var gradeReportJson string
		if err = rows.Scan(&solution, &elt.TimeStamp, &submissionJson, &gradeReportJson, &elt.Passed); err != nil {
			log.Fatalf("DB error scanning Submission: %v", err)
		}
		elt.Solution = solutionsByID[solution]
		if err = json.Unmarshal([]byte(submissionJson), &elt.Submission); err != nil {
			log.Fatalf("JSON error in Submission for Solution %d at %v: %v", elt.Solution.ID, elt.TimeStamp, err)
		}
		if gradeReportJson == "" {
			elt.GradeReport = make(map[string]interface{})

			// missing grade report? add this to the grading queue
			gradeQueue[solution] = true
		} else if err = json.Unmarshal([]byte(gradeReportJson), &elt.GradeReport); err != nil {
			log.Fatalf("JSON error in GradeReport for Solution %d at %v: %v", elt.Solution.ID, elt.TimeStamp, err)
		}
		elt.Solution.SubmissionsInOrder = append(elt.Solution.SubmissionsInOrder, elt)
	}
}
