package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

type GradeItem struct {
	SolutionID  int64
	ProblemType *ProblemType
	ProblemData map[string]interface{}
	Attempt     map[string]interface{}
}

var notifyGrader chan int64
var gradeQueue = make(map[int64]bool)

func gradeDaemon() {
	delay := time.Second
	retry := true
	for {
		if !retry {
			id := <-notifyGrader
			gradeQueue[id] = true
		}

		// clear out the channel
	clearLoop:
		for {
			select {
			case id := <-notifyGrader:
				gradeQueue[id] = true
			default:
				break clearLoop
			}
		}

		err := gradeAll()
		if err != nil {
			log.Printf("gradeDaemon err: sleeping for %v", delay)
			time.Sleep(delay)

			delay *= 2
			if delay > time.Minute {
				delay = time.Minute
			}

			retry = true
		} else {
			delay = time.Second
			retry = false
		}
	}
}

func gradeAll() (err error) {
	didwork := true
	for didwork && err == nil {
		didwork, err = gradeOne(database)
	}
	return
}

func gradeOne(db *sql.DB) (bool, error) {
	if len(gradeQueue) == 0 {
		return false, nil
	}

	// get an item to grade
	var id int64
	for id, _ = range gradeQueue {
		break
	}

	// get a read lock to retrieve the submission data
	mutex.RLock()
	solution, present := solutionsByID[id]
	if !present {
		log.Printf("gradeOne: no solution found with ID %d", id)
		delete(gradeQueue, id)
		mutex.RUnlock()
		return false, fmt.Errorf("no solution found with given ID")
	}

	// get the problem type
	asst := solution.Assignment
	problem := asst.Problem
	problemType := problem.Type

	// find the first ungraded submission
	var i int
	for i = len(solution.SubmissionsInOrder) - 1; i >= 0; i-- {
		if len(solution.SubmissionsInOrder[i].GradeReport) > 0 {
			break
		}
	}
	i++
	if i >= len(solution.SubmissionsInOrder) {
		delete(gradeQueue, id)
		mutex.RUnlock()
		return false, fmt.Errorf("No ungraded submissions")
	}
	attempt := solution.SubmissionsInOrder[i]

	log.Printf("Grading solution #%d (%d/%d) of type %s for %s",
		id, i+1, len(solution.SubmissionsInOrder), problem.Type.Tag, solution.Student.Email)

	// merge the fields into a single submission record
	merged := make(map[string]interface{})

	for _, field := range problemType.FieldList {
		if value, present := attempt.Submission[field.Name]; present && field.Grader == "view" {
			merged[field.Name] = value
		} else if value, present := problem.Data[field.Name]; present && field.Grader == "view" {
			merged[field.Name] = value
		}
	}

	// form the request json
	requestBody, err := json.Marshal(merged)
	if err != nil {
		log.Printf("gradeOne: error marshalling data for grader: %v", err)
		delete(gradeQueue, id)
		mutex.RUnlock()
		return false, err
	}

	// release the read mutex
	mutex.RUnlock()

	// send it to the grader
	u := &url.URL{
		Scheme: "http",
		Host:   config.GraderAddress,
		Path:   "/" + problemType.Tag,
	}
	request, err := http.NewRequest("POST", u.String(), bytes.NewReader(requestBody))
	if err != nil {
		log.Printf("gradeOne: error creating request object: %v", err)
		delete(gradeQueue, id)
		return false, err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Accept", "application/json")
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		log.Printf("gradeOne: error sending request to %s: %v", u.String(), err)
		return false, err
	}
	if resp.StatusCode != 200 {
		log.Printf("gradeOne: error result from request to %s: %s", u.String(), resp.Status)
		return false, err
	}
	defer resp.Body.Close()

	// decode the response
	report := make(map[string]interface{})

	if err = json.NewDecoder(resp.Body).Decode(&report); err != nil {
		log.Printf("gradeOne: failed to decode response from %s: %v", u.String(), err)
		return false, err
	}
	if len(report) == 0 {
		log.Printf("gradeOne: response list from %s is emtpy", u.String())
		return false, fmt.Errorf("Empty grader report")
	}
	if _, present = report["Passed"]; !present {
		log.Printf("gradeOne: response is missing Passed field")
		return false, fmt.Errorf("Missing Passed field")
	}

	// re-encode the response
	graderReportJson, err := json.Marshal(report)
	if err != nil {
		log.Printf("gradeOne: JSON error encoding grade report: %v", err)
		return false, fmt.Errorf("JSON error encoding grade report")
	}

	// record the response
	mutex.Lock()
	defer mutex.Unlock()

	solution = solutionsByID[id]
	if i >= len(solution.SubmissionsInOrder) || len(solution.SubmissionsInOrder[i].GradeReport) > 0 {
		log.Printf("gradeOne: submission changed during grading for %d", id)
		return false, fmt.Errorf("Submission change during grading")
	}
	sub := solution.SubmissionsInOrder[i]
	passed := false

	switch t := report["Passed"].(type) {
	case bool:
		passed = t
	default:
		log.Printf("gradeOne: Passed field of wrong type %t", t)
		return false, fmt.Errorf("Passed field of wrong type")
	}

	// write to database first
	_, err = database.Exec("update Submission set GradeReport = ?, Passed = ? where Solution = ? and TimeStamp = ?",
		graderReportJson, passed, sub.Solution.ID, sub.TimeStamp)
	if err != nil {
		log.Printf("gradeOne: DB error writing result: %v", err)
		return false, err
	}
	sub.GradeReport = report
	sub.Passed = passed

	// remove this solution from the queue?
	if i == len(solution.SubmissionsInOrder)-1 {
		delete(gradeQueue, id)
	}

	return true, nil
}
