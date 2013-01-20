Students
--------

*   Get a list of courses and open problems

        GET /student/list/courses

    Returns a list of active courses the student is enrolled in.
    Each contains:

    *   Name: the name of the course
    *   Close: timestamp when the course ends
    *   Instructors: list of instructor emails for this course
    *   OpenAssignments: list of open assignments for this course,
        sorted by deadline
    *   NextAssignment: generic listing for the next assignment to
        become available, or null if none is currently set

    Generic assignment listings contain the following:

    *   Name: the name of the problem
    *   ID: the ID# of the assignment
    *   Open: timestamp when the problem opens
    *   Close: timestamp when the problem closes
    *   ForCredit: false if this assignment is not required

    For active assignments, the listing also contains the following:

    *   Attempts: the number of attempts the student has made on
        this problem
    *   ToBeGraded: the number of attempts that have not yet been
        graded (attempts are always graded in order)
    *   Passed: true if the most recent attempt was successful

*   Get a grade report for a course

        GET /student/list/grades/COURSETAG

    Returns a list of assignments for the specified course and the
    student's result in each one. Each assignment in the list
    includes the generic and student-specific report. The list
    includes all assignments that are already closed and those that
    are currently open (but not those that will open in the future).

*   Get an open assignment

        GET /student/assignment/ID#

    Returns data about the given assignment, and the student's most
    recent attempt (if applicable):

    *   Assignment: assignment listing as in list/courses, with
        generic and student-specific data
    *   CourseName: name of the course
    *   CourseTag: tag for the course
    *   Type: problem type tag
    *   FieldList: list of fields for the problem type
    *   Problem: contents of the problem
    *   Attempt: the student's most recent attempt (if applicable)

*   Submit an assignment attempt

        POST /student/attempt/ID#

    Saves an attempted solution for assignement ID#. This only fails
    if there is an invalid submission. Whether or not the
    submission passed is not immediately available. The result can
    be found by polling the course list or the student's grade list.

    The submissions includes a JSON payload with the student's
    attempt.


Instructors
-----------

*   Upload a Canvas CSV course grade list to update course
    membership:

        POST /instructor/update/courselist

    The CSV file must be included as the form field `CSVFile`.
    Returns the following:

    *   Success: true if the update was successful. False may
        indicate a partial update.
    *   UnknownCanvasCourseTag: if set, the CSV file contained a
        canvas course name that is not currently mapped to a course tag.
    *   UnknownStudents: a list of students (as displayable strings)
        found in the CSV file for which email addresses are unknown.
        The mappings must be updated before the students can be
        enrolled in the course.
    *   PossibleDrops: a list of student pairs (each containing Name
        and Email fields) for students that are currently enrolled
        in the course but were not found in the CSV file.
    *   Log: a list of information messages created as the file was
        processed.

*   Update some course and student mapping data:

        POST /instructor/update/canvasmappings

    The contents must be JSON data containing the following:

    *   CourseCanvasToTag: an object with mappings from canvas
        course IDs to course tags.
    *   StudentIDToEmail: an object with mappings from canvas
        student IDs to student email addresses.

    Once these mappings are updated, the CSV file can be uploaded
    again to finish updating course membership.


Problems
--------

*   Get a list of problem types

        GET /problem/listtypes

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
    *   Editor: action to take when presenting this field to a
        problem editor. One of {view, edit}
    *   Student: action to take when presenting this field to a
        student. Same options as for Editor

*   Get a problem type description

        GET /problem/type/TAG

    Returns a description of the problem type named by TAG. Format
    is same as for listtypes.

*   Create a new problem (instructor)

        POST /problem/create

    Create a new problem (not an assignment), with the following
    data:

    *   Name: a human-readable name for this problem
    *   Type: the evaluation type of the problem
    *   Tags: a list of tags to help with finding this problem later
    *   Problem: contents of the problem

    Returns the newly-created problem object.

