package main

import (
	"bufio"
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

const scriptPath = "scripts"

func main() {
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
	if err := loadScripts(db, scriptPath); err != nil {
		log.Fatal("Loading Lua scripts: ", err)
	}

	shell(db)
}

func loadScripts(db *redis.Client, path string) (err error) {
	names, err := filepath.Glob(path + "/*.lua")
	if err != nil {
		return
	}

	for _, name := range names {
		_, key := filepath.Split(name)
		key = key[:len(key)-len(".lua")]
		log.Printf("loading script %s", key)

		var contents []byte
		if contents, err = ioutil.ReadFile(name); err != nil {
			return
		}

		var reply *redis.Reply
		if reply = db.Call("script", "load", contents); reply.Err != nil {
			return
		}

		if luaScripts[key], err = reply.Str(); err != nil {
			return
		}
	}

	return
}

func shell(db *redis.Client) {
	log.Print("Codrilla interactive shell")
	log.Print("Type \"help\" for a list of recognized commands")

	in := bufio.NewReader(os.Stdin)

mainloop:
	for {
		// get a line of input
		fmt.Print("> ")
		line, err := in.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Print("Error reading from keyboard: ", err)
			}
			break
		}

		conf := &scanner.Scanner{}
		s := conf.Init(strings.NewReader(line))
		tok := s.Scan()
		if tok == scanner.EOF {
			continue
		}
		if tok != scanner.Ident {
			log.Print("Invalid command, type \"help\" for a list of commands")
			continue
		}

		switch s.TokenText() {
		case "help":
			log.Print("  help: see this list")
			log.Print("  list instructors")
			log.Print(`  create instructor`)
			log.Print(`    "prof@dixie.edu (email)"`)
			log.Print(`    "First Last (name)"`)
			log.Print(`  create course`)
			log.Print(`    "cs1410-s13-10am (tag)"`)
			log.Print(`    "CS 1410: Object Oriented Programming (Spring 2013, MWF 10AM)"`)
			log.Print(`    "May 15, 2013 (closing time)"`)
			log.Print(`    "prof@dixie.edu (instructor)"`)
			log.Print("  exit")
			log.Print("  quit")

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
			if s.Scan() != scanner.EOF {
				log.Print("Extra stuff at end of list command")
				continue
			}
			switch text {
			case "instructors":
				cmd_listinstructors(db)

			default:
				log.Print("List commands: instructors")
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

		default:
			log.Print("Invalid command, type \"help\" for a list of commands")
			continue
		}
	}
	log.Print("Bye")
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
	email, err := getValidEmail(s)
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
	email, err := getValidEmail(s)
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

func getValidEmail(s *scanner.Scanner) (email string, err error) {
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
	if !strings.HasSuffix(email, "@dixie.edu") {
		log.Print("Email must be @dixie.edu")
		return "", fmt.Errorf("Must be @dixie.edu")
	}
	if strings.Count(email, "@") != 1 {
		log.Print("Email can only contain one @ character")
		return "", fmt.Errorf("Must contain only one @ character")
	}

	return
}

func getValidTag(s *scanner.Scanner) (tag string, err error) {
	if tok := s.Scan(); tok != scanner.String {
		log.Print("Invalid string for tag name")
		return
	}
	tag, err = strconv.Unquote(s.TokenText())
	if err != nil {
		log.Printf("String error for tag name: %v", err)
		return "", err
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
	if tok := s.Scan(); tok != scanner.String {
		log.Printf("Did not find string for %s field", fieldName)
		return "", fmt.Errorf("String not found")
	}
	elt, err = strconv.Unquote(s.TokenText())
	if err != nil {
		log.Printf("String error for %s field: %v", fieldName, err)
		return "", err
	}
	elt = strings.TrimSpace(elt)
	if len(elt) == 0 {
		log.Printf("%s field cannot be an empty string", fieldName)
		return
	}

	return
}
