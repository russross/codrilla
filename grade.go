package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
	"net/url"
	"strconv"
)

type GradeItem struct {
	SolutionID  int64
	ProblemType *ProblemType
	ProblemData map[string]interface{}
	Attempt     map[string]interface{}
}

func gradeOne(db *redis.Client) (bool, error) {
	// get an item to grade
	iface := db.EvalSha(luaScripts["gradeget"], nil, []string{})
	if iface.Err() != nil {
		log.Printf("gradeOne: DB error getting item to grade: %v", iface.Err())
		return false, iface.Err()
	}
	raw := iface.Val().(string)

	// was there nothing to do?
	if raw == "" {
		return false, nil
	}

	// parse the item to grade
	item := new(GradeItem)
	if err := json.Unmarshal([]byte(raw), item); err != nil {
		log.Printf("gradeOne: Unable to parse submission data: %v", err)
		return false, err
	}

	log.Printf("Grading solution #%d", item.SolutionID)

	// merge the fields into a single submission record
	merged := make(map[string]interface{})

	for _, field := range item.ProblemType.FieldList {
		if value, present := item.Attempt[field.Name]; present && field.Student == "edit" && field.Grader == "view" {
			merged[field.Name] = value
		} else if value, present := item.ProblemData[field.Name]; present && field.Creator == "edit" && field.Grader == "view" {
			merged[field.Name] = value
		}
	}

	// form the request json
	requestBody, err := json.Marshal(merged)
	if err != nil {
		log.Printf("gradeOne: error marshalling data for grader: %v", err)
		return false, err
	}

	// send it to the grader
	u := &url.URL{
		Scheme: "http",
		Host:   config.GraderAddress,
		Path:   "/" + item.ProblemType.Tag,
	}
	resp, err := http.Post(u.String(), "application/json", bytes.NewReader(requestBody))
	if err != nil {
		log.Printf("gradeOne: error sending request to %s: %v", u.String(), err)
		return false, err
	}
	if resp.StatusCode != 200 {
		log.Printf("gradeOne: error result from request to %s: %v", u.String(), err)
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

	// re-encode the report
	data, err := json.Marshal(report)
	if err != nil {
		log.Printf("gradeOne: error JSON encoding final report: %v", err)
		return false, err
	}

	// record the response
	id := strconv.FormatInt(item.SolutionID, 10)
	if iface = db.EvalSha(luaScripts["gradeput"], nil, []string{id, string(data)}); iface.Err() != nil {
		log.Printf("gradeOne: DB error saving graded item: %v", iface.Err())
		return false, iface.Err()
	}

	return true, nil
}
