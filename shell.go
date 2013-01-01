package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"github.com/russross/radix/redis"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/scanner"
	"time"
	"unicode"
)

// preferred time zone
var tz *time.Location

// map from filenames to sha1 hashes of all scripts that are loaded
var luaScripts = make(map[string]string)

// global stdin reader
var stdin *bufio.Reader

const scriptPath = "scripts"
const studentEmailSuffix = "@dmail.dixie.edu"
const instructorEmailSuffix = "@dixie.edu"

func shellmain() {
	// initialize time
	zone, err := time.LoadLocation("America/Denver")
	if err != nil {
		log.Fatal("Loading time zone: ", err)
	}
	tz = zone

	// connect to redis
	config := redis.DefaultConfig()
	db := redis.NewClient(config)

	// set up Lua scripts
	log.Print("Codrilla interactive shell")
	log.Println()
	loadScripts(db, scriptPath)
	shell(db)
}

func loadScripts(db *redis.Client, path string) {
	names, err := filepath.Glob(path + "/*.lua")
	if err != nil {
		log.Fatalf("Failed to get list of Lua scripts: %v", err)
	}

	count := 0
	for _, name := range names {
		_, key := filepath.Split(name)
		key = key[:len(key)-len(".lua")]

		var contents []byte
		if contents, err = ioutil.ReadFile(name); err != nil {
			log.Fatalf("Failed to load script %s: %v", name, err)
		}

		var reply *redis.Reply
		if reply = db.Call("script", "load", contents); reply.Err != nil {
			log.Fatalf("Failed to load script %s into redis: %v", name, err)
		}

		if luaScripts[key], err = reply.Str(); err != nil {
			log.Fatalf("DB error loading script %s: %v", name, err)
		}
		count++
	}
	log.Printf("Loaded %d Lua scripts", count)
}

func cron(db *redis.Client) {
	now := time.Now().Unix()
	if _, err := db.Call("evalsha", luaScripts["cron"], 0, now).Str(); err != nil {
		log.Printf("Error running cron job: %v", err)
	}
}

func shell(db *redis.Client) {
	log.Print("Type \"help\" for a list of recognized commands")

	stdin = bufio.NewReader(os.Stdin)

mainloop:
	for {
		// get a line of input
		s, err := prompt("> ")
		if err != nil {
			break
		}
		tok := s.Scan()
		if tok == scanner.EOF {
			continue
		}
		if tok != scanner.Ident {
			log.Print("Invalid command, type \"help\" for a list of commands")
			continue
		}

		cron(db)

		switch s.TokenText() {
		case "help":
			fmt.Println(`* help: see this list`)
			fmt.Println(`* list instructors`)
			fmt.Println(`* list course "cs1410-s13-10am"`)
			fmt.Println(`* create instructor`)
			fmt.Println(`    "prof@dixie.edu (email)"`)
			fmt.Println(`    "First Last (name)"`)
			fmt.Println(`* create course`)
			fmt.Println(`    "cs1410-s13-10am (tag)"`)
			fmt.Println(`    "CS 1410: Object Oriented Programming (Spring 2013, MWF 10am)`)
			fmt.Println(`    "May 15, 2013 (closing time)"`)
			fmt.Println(`    "prof@dixie.edu (instructor)"`)
			fmt.Println(`* update course "Grades-course.csv: update membership`)
			fmt.Println(`* remove "stud@dmail.dixie.edu" "cs1410-s13-10am":`)
			fmt.Println(`    remove student from course`)
			fmt.Println(`* exit`)
			fmt.Println(`* quit`)

		case "exit":
			break mainloop

		case "quit":
			break mainloop

		case "list":
			if tok = s.Scan(); tok != scanner.Ident {
				log.Print("List commands: instructors")
				continue
			}
			text := s.TokenText()
			switch text {
			case "instructors":
				cmd_listinstructors(db)

			case "course":
				cmd_listcourse(db, s)

			default:
				log.Print("List commands: instructors")
			}
			if s.Scan() != scanner.EOF {
				log.Print("Extra stuff at end of list command")
				continue
			}

		case "create":
			if tok = s.Scan(); tok != scanner.Ident {
				log.Print("Create commands: instructor, course")
				continue
			}
			switch s.TokenText() {
			case "instructor":
				cmd_createinstructor(db, s)

			case "course":
				cmd_createcourse(db, s)

			default:
				log.Print("Create commands: instructor, course")
			}

		case "remove":
			cmd_removestudent(db, s)

		case "update":
			if tok = s.Scan(); tok != scanner.Ident {
				log.Print("Update commands: course")
				continue
			}
			switch s.TokenText() {
			case "course":
				cmd_updatecourse(db, s)

			default:
				log.Print("Update commands: course")
			}

		default:
			log.Print("Invalid command, type \"help\" for a list of commands")
			continue
		}
	}
	log.Print("Bye")
}

func cmd_removestudent(db *redis.Client, s *scanner.Scanner) {
	// get email address
	email, err := getValidEmail(s, studentEmailSuffix)
	if err != nil {
		return
	}

	// get course tag
	tag, err := getNonEmptyString(s, "course tag")
	if err != nil {
		return
	}

	_, err = db.Call("evalsha", luaScripts["removestudentfromcourse"], 0, email, tag).Str()
	if err != nil {
		log.Printf("DB error removing student: %v", err)
	}
}

func cmd_updatecourse(db *redis.Client, s *scanner.Scanner) {
	// get the file name for the CSV file
	filename, err := getNonEmptyString(s, "file name")
	if err != nil {
		return
	}

	// open and parse the file
	fp, err := os.Open(filename)
	if err != nil {
		log.Printf("Error opening CSV file: %v", err)
		return
	}
	defer fp.Close()
	reader := csv.NewReader(fp)
	reader.TrailingComma = true
	records, err := reader.ReadAll()
	if err != nil {
		log.Printf("Error parsing CSV file: %v", err)
		return
	}
	if len(records) < 3 {
		log.Printf("File does not seem to contain any students")
		return
	}

	// throw away the header lines
	records = records[2:]

	// create a student record for each one
	students := make(map[string]string)
	course := ""
	for _, student := range records {
		if len(student) < 3 {
			log.Printf("Student record with too few fields: %v", student)
			return
		}
		name, id, section := student[0], student[1], student[2]

		// make sure this is a known course
		exists, err := db.Hexists("index:courses:tagbycanvastag", section).Bool()
		if err != nil {
			log.Printf("Error checking for course tag for %s: %v", section, err)
			return
		}
		if !exists {
			// prompt for the dmail address to go with this student
			line, err := prompt(fmt.Sprintf("Course tag for %s? ", section))
			if err != nil {
				return
			}

			tag, err := getNonEmptyString(line, "tag")
			if err != nil {
				return
			}

			exists, err := db.Sismember("index:courses:active", tag).Bool()
			if err != nil {
				log.Printf("Error checking for course %s: %v", tag, err)
				return
			}
			if !exists {
				log.Printf("%s is not an active course", tag)
				return
			}

			_, err = db.Hset("index:courses:tagbycanvastag", section, tag).Int()
			if err != nil {
				log.Printf("Error setting mapping of Canvas ID -> tag: %v", err)
				return
			}
		}
		tag, err := db.Hget("index:courses:tagbycanvastag", section).Str()
		if err != nil {
			log.Printf("Error getting tag for course %s: %v", section, err)
			return
		}
		if course == "" {
			course = tag
		} else if course != "" && tag != course {
			log.Printf("Error: two courses found, %s and %s", course, tag)
			return
		}

		// see if we know the email address for this student
		exists, err = db.Hexists("index:students:emailbyid", id).Bool()
		if err != nil {
			log.Printf("Error checking for student ID %s for student %s: %v", id, name, err)
			return
		}
		if !exists {
			// prompt for the dmail address to go with this student
			line, err := prompt(fmt.Sprintf("Email (NOT include @dmail.dixie.edu) for %s (%s)? ", name, id))
			if err != nil {
				return
			}

			email, err := getNonEmptyString(line, "email")
			if err != nil {
				return
			}

			// sanity check
			email = strings.ToLower(email)
			for _, ch := range email {
				if !unicode.IsLower(ch) && !unicode.IsDigit(ch) {
					log.Printf("Invalid character found in email: %#v", ch)
					return
				}
			}
			email = email + studentEmailSuffix

			// write an entry to the db
			_, err = db.Hset("index:students:emailbyid", id, email).Int()
			if err != nil {
				log.Printf("Error setting id -> email mapping: %v", err)
			}
		}

		// get the email address
		email, err := db.Hget("index:students:emailbyid", id).Str()
		if err != nil {
			log.Printf("Error getting student email address for %s (%s): %v", name, id, err)
			return
		}

		// note the students we have found
		students[email] = name

		// add this student to the course
		result, err := db.Call("evalsha", luaScripts["addstudenttocourse"], 0, email, name, tag).Str()
		if err != nil {
			log.Printf("DB error adding student to course: %v", err)
			return
		}

		if result == "noop" {
			log.Printf("Student %s (%s) skipped", name, email)
		} else {
			log.Printf("Student %s (%s) added to %s", name, email, tag)
		}
	}
	if course == "" {
		log.Printf("Error: no course found")
		return
	}

	// check for students that were not in the list
	roll, err := db.Smembers("course:" + course + ":students").List()
	if err != nil {
		log.Printf("Error getting list of students in course %s: %v", course, err)
		return
	}
	for _, elt := range roll {
		if _, present := students[elt]; !present {
			log.Printf("Warning: student %s is in %s but not in this CSV file", elt, course)
		}
	}
}

func prompt(s string) (*scanner.Scanner, error) {
	fmt.Print(s)
	line, err := stdin.ReadString('\n')
	if err != nil {
		if err != io.EOF {
			log.Print("Error reading from keyboard: ", err)
		}
		return nil, err
	}

	conf := &scanner.Scanner{}
	return conf.Init(strings.NewReader(line)), nil
}

func cmd_listcourse(db *redis.Client, s *scanner.Scanner) {
	// get the course tag
	tag, err := getNonEmptyString(s, "course tag")
	if err != nil {
		return
	}

	// is it a valid course?
	active, err := db.Sismember("index:courses:active", tag).Bool()
	if err != nil {
		log.Printf("DB error checking for active course %s: %v", tag, err)
		return
	}
	inactive, err := db.Sismember("index:courses:inactive", tag).Bool()
	if err != nil {
		log.Printf("DB error checking for inactive course %s: %v", tag, err)
		return
	}
	if active || inactive {
		name, err := db.Get("course:" + tag + ":name").Str()
		if err != nil {
			log.Printf("DB error getting name of course: %v", err)
			return
		}
		fmt.Println(name)
		unix, err := db.Get("course:" + tag + ":close").Int64()
		if err != nil {
			log.Printf("DB error getting closing timestamp: %v", err)
			return
		}
		now := time.Now().In(tz)
		closetime := time.Unix(unix, 0).In(tz)
		if active {
			fmt.Printf("Course will close at %v (%v)\n", closetime, closetime.Sub(now))
		} else {
			fmt.Printf("Course closed at %v\n", closetime)
		}
	} else {
		log.Print("Unknown course")
		return
	}

	// list the active assignments
	
}

func cmd_listinstructors(db *redis.Client) {
	set, err := db.Smembers("index:instructors:active").List()
	if err != nil {
		log.Printf("DB error: %v", err)
		return
	}
	if len(set) > 0 {
		log.Print("Instructors with active courses:")
	}
	for _, email := range set {
		name, err := db.Get("instructor:" + email + ":name").Str()
		if err != nil {
			log.Printf("DB error getting name for %s: %v", email, err)
			return
		}
		courses, err := db.Smembers("instructor:" + email + ":courses").List()
		if err != nil {
			log.Printf("DB error getting courses for %s: %v", email, err)
			return
		}
		sort.Strings(courses)
		log.Printf("  %s (%s): %s", name, email, strings.Join(courses, ", "))
	}

	set, err = db.Smembers("index:instructors:inactive").List()
	if err != nil {
		log.Printf("DB error: %v", err)
		return
	}
	if len(set) > 0 {
		log.Print("Instructors with no active courses:")
	}
	for _, email := range set {
		name, err := db.Get("instructor:" + email + ":name").Str()
		if err != nil {
			log.Printf("DB error getting name for %s: %v", email, err)
			return
		}
		log.Printf("  %s (%s)", name, email)
	}
}

func cmd_createinstructor(db *redis.Client, s *scanner.Scanner) {
	// createinstructor "email@dixie.edu" "Full Name"

	// get email
	email, err := getValidEmail(s, instructorEmailSuffix)
	if err != nil {
		return
	}

	// get full name
	name, err := getNonEmptyString(s, "full name")
	if err != nil {
		return
	}
	name = strings.Title(name)

	log.Printf("Creating instructor %s (%s)", email, name)
	_, err = db.Call("evalsha", luaScripts["createinstructor"], 0, email, name).Str()
	if err != nil {
		log.Printf("DB error: %v", err)
	}
}

func cmd_createcourse(db *redis.Client, s *scanner.Scanner) {
	// createcourse "CS1410" "Object OP" "5/16/2013" "russ@dixie.edu"

	// get tag
	tag, err := getValidTag(s)
	if err != nil {
		return
	}

	// get full name
	name, err := getNonEmptyString(s, "full name")
	if err != nil {
		return
	}

	// get closing time stamp
	closetime, err := parseTime(s)
	if err != nil {
		log.Print("Unable to parse time stamp for closing time")
		return
	}

	// get instructor email
	email, err := getValidEmail(s, instructorEmailSuffix)
	if err != nil {
		return
	}

	// stray arguments?
	if tok := s.Scan(); tok != scanner.EOF {
		log.Print("Extra arguments found")
		return
	}

	log.Printf("Creating course %#v (%#v) ends %v taught by %v", tag, name, closetime, email)

	_, err = db.Call("evalsha", luaScripts["createcourse"], 0, tag, name, closetime.Unix(), email).Str()
	if err != nil {
		log.Printf("DB error: %v", err)
	}
}

// try parsing a timestamp using a few common formats
func parseTime(s *scanner.Scanner) (t time.Time, err error) {
	// get time stamp
	if tok := s.Scan(); tok != scanner.String {
		log.Print("Invalid string for time stamp")
		return
	}
	raw, err := strconv.Unquote(s.TokenText())
	if err != nil {
		log.Printf("String error for time stamp: %v", err)
		return
	}

	dayonly := false

	if t, err = time.Parse("Jan 2, 2006", raw); err == nil {
		dayonly = true
	} else if t, err = time.Parse("Jan 2, 2006 3:04PM", raw); err == nil {
	} else if t, err = time.Parse("Jan 2, 2006 3:04 PM", raw); err == nil {
	} else if t, err = time.Parse("Jan 2, 2006 15:04", raw); err == nil {

	} else if t, err = time.Parse("Jan 2 2006", raw); err == nil {
		dayonly = true
	} else if t, err = time.Parse("Jan 2 2006 3:04PM", raw); err == nil {
	} else if t, err = time.Parse("Jan 2 2006 3:04 PM", raw); err == nil {
	} else if t, err = time.Parse("Jan 2 2006 15:04", raw); err == nil {

	} else if t, err = time.Parse("1/2/2006", raw); err == nil {
		dayonly = true
	} else if t, err = time.Parse("1/2/2006 3:04PM", raw); err == nil {
	} else if t, err = time.Parse("1/2/2006 3:04 PM", raw); err == nil {
	} else if t, err = time.Parse("1/2/2006 15:04", raw); err == nil {

	} else if t, err = time.Parse("1-2-2006", raw); err == nil {
		dayonly = true
	} else if t, err = time.Parse("1-2-2006 3:04PM", raw); err == nil {
	} else if t, err = time.Parse("1-2-2006 3:04 PM", raw); err == nil {
	} else if t, err = time.Parse("1-2-2006 15:04", raw); err == nil {

	} else if t, err = time.Parse("2006-1-2", raw); err == nil {
		dayonly = true
	} else if t, err = time.Parse("2006-1-2 03:04PM", raw); err == nil {
	} else if t, err = time.Parse("2006-1-2 03:04 PM", raw); err == nil {
	} else if t, err = time.Parse("2006-1-2 15:04", raw); err == nil {
	}

	if err != nil {
		return
	}

	if dayonly {
		t = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, tz)
	} else {
		t = time.Date(t.Year(), t.Month(), t.Day(),
			t.Hour(), t.Minute(), t.Second(), 0, tz)
	}

	// make sure the time is sane
	if t.Before(time.Now()) {
		log.Print("Time stamp is in the past")
		err = fmt.Errorf("Time stamp is in the past")
		return
	}
	if t.After(time.Now().Add(time.Hour * 24 * 366)) {
		log.Print("Time stamp is more than a year in the future")
		err = fmt.Errorf("Time stamp is more than a year in the future")
		return
	}

	return
}

func getValidEmail(s *scanner.Scanner, suffix string) (email string, err error) {
	// get email
	if tok := s.Scan(); tok != scanner.String {
		log.Print("Invalid string for email")
		return "", fmt.Errorf("Invalid string for email")
	}
	email, err = strconv.Unquote(s.TokenText())
	if err != nil {
		log.Printf("String error for email: %v", err)
		return "", err
	}
	email = strings.ToLower(email)
	for _, ch := range email {
		if !unicode.IsLower(ch) && !unicode.IsNumber(ch) && ch != '.' && ch != '@' {
			log.Print("Email must only contain letters, digits, ., and @ characters")
			return "", fmt.Errorf("Invalid character in email")
		}
	}
	if !strings.HasSuffix(email, suffix) {
		log.Printf("Email must be %s", suffix)
		return "", fmt.Errorf("Must be %s", suffix)
	}
	if strings.Count(email, "@") != 1 {
		log.Print("Email can only contain one @ character")
		return "", fmt.Errorf("Must contain only one @ character")
	}

	return
}

func getValidTag(s *scanner.Scanner) (tag string, err error) {
	tok := s.Scan()
	if tok != scanner.String && tok != scanner.Ident {
		log.Print("Invalid string for tag name")
		return
	}
	tag = s.TokenText()
	if tok == scanner.String {
		tag, err = strconv.Unquote(tag)
		if err != nil {
			log.Printf("String error for tag name: %v", err)
			return "", err
		}
	}
	if len(tag) == 0 {
		log.Print("Tag cannot be an empty string")
		return "", fmt.Errorf("Tag cannot be an empty string")
	}
	for _, ch := range tag {
		if !unicode.IsLower(ch) && !unicode.IsNumber(ch) && ch != '-' && ch != '_' {
			log.Print("Tag must only contain lower case letters, digits, -, and _")
			return "", fmt.Errorf("Tag has invalid characters: lower, digits, -, and _ allowed")
		}
	}

	return
}

func getNonEmptyString(s *scanner.Scanner, fieldName string) (elt string, err error) {
	tok := s.Scan()
	if tok != scanner.String && tok != scanner.Ident {
		log.Printf("Did not find string for %s field", fieldName)
		return "", fmt.Errorf("String not found")
	}
	elt = s.TokenText()
	if tok == scanner.String {
		elt, err = strconv.Unquote(elt)
		if err != nil {
			log.Printf("String error for %s field: %v", fieldName, err)
			return "", err
		}
	}
	elt = strings.TrimSpace(elt)
	if len(elt) == 0 {
		log.Printf("%s field cannot be an empty string", fieldName)
		return
	}

	return
}
