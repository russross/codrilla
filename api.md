Students
--------

*   Get a list of courses and open problems

        GET /student/courses

    Returns some basic info about the student as well as a list of
    active courses the student is enrolled in. Fields include:
    
    *   Email: the student's email address
    *   Name: the name of the student
    *   Courses: a list of course
    
    Each course contains:

    *   Tag: the course tag
    *   Name: the name of the course
    *   Close: timestamp when the course ends
    *   Instructors: list of instructor emails for this course
    *   OpenAssignments: list of open assignments for this course,
        sorted by deadline
    *   NextAssignment: generic listing for the next assignment to
        become available, or null if none is currently set

    Generic assignment listings contain the following:

    *   ID: the ID# of the assignment
    *   Name: the name of the problem
    *   Open: timestamp when the problem opens
    *   Close: timestamp when the problem closes
    *   Active: true if the assignment is currently open
    *   ForCredit: false if this assignment is not required

    The listing also contains the following, which may be blank when
	not applicable:

    *   Attempts: the number of attempts the student has made on
        this problem
    *   ToBeGraded: the number of attempts that have not yet been
        graded (attempts are always graded in order)
    *   Passed: true if the most recent attempt was successful

*   Get a grade report for a course

        GET /student/grades/COURSETAG

    Returns a list of assignments for the specified course and the
    student's result in each one. Each assignment in the list
    includes the generic and student-specific report. The list
    includes all assignments that are already closed and those that
    are currently open (but not those that will open in the future).

*   Get an open assignment

        GET /student/assignment/ID#

    Returns data about the given assignment, and the student's most
    recent attempt (if applicable):

    *   CourseTag: tag for the course
    *   CourseName: name of the course
    *   ProblemType: a problem type object:

        *   Tag: problem type tag
        *   Name: name of the problem type
        *   FieldList: list of fields for the problem type

    *   ProblemData: contents of the problem
    *   Assignment: assignment listing as in list/courses, with
        generic and student-specific data
    *   Attempt: the student's most recent attempt (if applicable)

*   Submit an assignment attempt

        POST /student/submit/ID#

    Saves an attempted solution for assignement ID#. This only fails
    if there is an invalid submission. Whether or not the
    submission passed is not immediately available. The result can
    be found by polling the course list or the student's grade list.

    The request includes a JSON payload with the student's
    attempt.

*   Get submission feedback

        GET /student/result/ID#/N

    Retrieves a submission result for assignement ID#. Retrieves
    attempt number N. If N is -1, it retrieves the most recent
    attempt. 

	This returns the same data as /student/assignement, with the
	"Passed" field removed and an additional field added:

	*   ResultData: contents of the report from the grader. This is
		empty or missing if the attempt has not been graded yet.

	The "Attempt" field is also the attempt requested, not
	necessarily the most recent attempt.


Courses
-------

*   Upload a Canvas CSV course grade list to update course
    membership:

        POST /course/canvascsvlist

    Up to 3 CSV files may be uploaded as form fields `CSVFile1`,
    `CSVFile2`, and `CSVFile3`, respectively.
    Returns the following:

    *   Success: true if the update was successful. False may
        indicate a partial update.
    *   UnknownStudents: a list of students (as displayable strings)
        found in the CSV file for which email addresses are unknown.
        The mappings must be updated before the students can be
        enrolled in the course.
    *   Log: a list of information messages created as the file was
        processed.

    All students enrolled in the course that do not appear in one of
    the CSV files will be dropped from the course.

*   Update and student mapping data:

        POST /course/canvasstudentmappings

    The contents must be JSON object mapping Canvas student IDs to
    email addresses.

*   Get a list of courses and assignments (instructor)

        GET /course/list

    Returns a list of active courses for this instructor. Each
    contains:

    *   Name: the name of the course
    *   Close: timestamp when the course ends
    *   Instructors: list of instructor emails for this course
    *   Students: list of student emails for this course
    *   OpenAssignments: list of open assignments for this course,
        sorted by deadline
    *   ClosedAssignments: list of closed assignments for this course,
        sorted by deadline
    *   FutureAssignments: list of future assignments for this
        course, sorted by open time

    Assignment lists contain generic assignment listings

*   Create an assignment

        POST /course/newassignment/COURSETAG

    Create a new assignment for the course indicated. Contents are
    JSON data containing:

    *   Problem: ID number of the problem
    *   Open: timestamp when the problem should open (optional--if
        missing, current time is used)
    *   Close: timestamp when the problem should close (must be in
        future)
    *   ForCredit: true if this assignment counts toward a grade

*   Get grades for all students in a course

        GET /course/grades/COURSETAG

    Returns a list of students with grades. Each contains:

    *   Name: student name
    *   Email: student email
    *   Assignments: a list of assignments. The list is the same as
        for the student grade report. Each element contains the
        generic and student-specific report for assignments that are
        open or closed (but not future).


Problems
--------

*   Get a list of problem types (instructor)

        GET /problem/types

    Returns a list of problem type descriptions. Each has the
    following:

    *   Name: the full name of the problem type
    *   Tag: the url-name of the problem type
    *   FieldList: a list of fields in order

    Each field is an object containing:

    *   Name: the JSON key used for this field
    *   Prompt: string displayed to anyone editing this field
    *   Title: string displayed to anyone viewing this field
    *   Type: string naming the field type. One of
        {markdown, python, text, string, int}
    *   List: true if this field is a list of elements of the same
        type
    *   Default: optional default value for this field (or list
        element)
    *   Creator: action to take when presenting this field to a
        problem editor. One of {view, edit, nothing}
    *   Student: action to take when presenting this field to a
        student. Same options as for Creator
    *   Grader: action to take when presenting this field to a
        grader. Same options as for Creator
    *   Result: action to take when presenting this field as a
        result. Same options as for Creator

*   Get a problem type description (instructor)

        GET /problem/type/TAG

    Returns a description of the problem type named by TAG. Format
    is same as for listtypes.

*   Create a new problem (instructor)

        POST /problem/new

    Create a new problem (not an assignment), with the following
    data:

    *   Name: a human-readable name for this problem
    *   Type: the evaluation type of the problem
    *   Tags: a list of tags to help with finding this problem later
    *   Data: contents of the problem

    Returns the newly-created problem object.

*   Save changes to a problem

        POST /problem/update/ID

    Same as for /problem/create, but updates an existing problem

*   Load a problem for preview/editing (instructor)

        GET /problem/get/ID

    Loads a problem object.

*   Get a list of problem tags (instructor)

        GET /problem/tags

    Returns the following:

    *   Tags: the list of tags, in order from high priority to low
    *   Problems: all problems, mapped by ID to problem object

    Each tag contains:

    *   Tag: the tag
    *   Description: the friendly description of the tag
    *   Priority: int from 0 to 100, higher being more important
    *   Problems: list of problem IDs with this tag

    Problems in the object are mapped by ID, and each contains:

    *   ID: the problem ID
    *   Name: problem name
    *   Type: evaluation type (a problem type tag)
    *   Tags: a list of all tags for this problem
    *   UsedBy: a list of courses that have used/are using this
        (list of course tags)

