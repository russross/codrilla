jQuery(function ($) {
    // get an approximation of the server's time
    var skew = 0;
    var serverTime = function () {
        var local = new Date().getTime();
        return new Date(local + skew);
    };

    var getCookies = function () {
        // note: CODRILLA is global
        CODRILLA = {
              Email: '',
              Role: '',
              Expires: 0,
              LoggedIn: false,
              LoginMethod: 'google'
        };
        var n = Number($.cookie('codrilla-expires'));
        CODRILLA.Expires = new Date(n * 1000);
        var now = serverTime();
        if (CODRILLA.Expires > now) {
            CODRILLA.Email = $.cookie('codrilla-email');
            CODRILLA.Role = $.cookie('codrilla-role');
            CODRILLA.LoggedIn = true;
        } else {
            CODRILLA.Email = null;
            CODRILLA.Role = null;
            CODRILLA.LoggedIn = false;
        }
    };

    getCookies();

    // login handling
    if (CODRILLA.LoginMethod == 'persona') {
        navigator.id.watch({
            loggedInUser: CODRILLA.Email,
            onlogin: function(assertion) {
                $.ajax({
                    type: 'POST',
                    url: '/auth/login/browserid',
                    dataType: 'json',
                    data: { Assertion: assertion },
                    success: function (res, status, xhr) {
                        getCookies();
                        setupLoggedIn();
                    },
                    error: function (res, status, xhr) {
                        console.log('login failure');
                        console.log(res);
                        setupLoggedOut();
                    }
                });
            },
            onlogout: function() {
                setupLoggedOut();
                $.ajax({
                    type: 'POST',
                    url: '/auth/logout',
                    success: function(res, status, xhr) {
                        setupLoggedOut();
                    },
                    error: function(res, status, xhr) {
                        console.log('logout failure');
                        console.log(res);
                        setupLoggedOut();
                    }
                });
            } 
        });
    }
    $('#persona-login-button').click(function () {
        navigator.id.request();
        return false;
    });
    $('#google-login-button').click(function () {
        var url = 'https://accounts.google.com/o/oauth2/auth' +
            '?response_type=code' +
            '&client_id=854211025378.apps.googleusercontent.com' +
            '&redirect_uri=http://' + window.location.host + '/auth/login/google' +
            '&scope=https://www.googleapis.com/auth/userinfo.email';
        var loginwindow = window.open(url, 'login');
        if (window.focus) loginwindow.focus();
        return false;
    });
    $('#logout-button').click(function () {
        if (CODRILLA.LoginMethod == 'persona')
            navigator.id.logout();
        else {
            $.ajax({
                type: 'POST',
                url: '/auth/logout',
                success: function(res, status, xhr) {
                    setupLoggedOut();
                },
                error: function(res, status, xhr) {
                    console.log('logout failure');
                    console.log(res);
                    setupLoggedOut();
                }
            });
        }
        return false;
    });

    var setupLoggedIn = function () {
        $('#loggedin').show();
        $('#loggedin-as').text(CODRILLA.Email);
        $('#notloggedin').hide();

        if (CODRILLA.Role == 'student')
            setupStudent();
        else if (CODRILLA.Role == 'instructor')
            setupInstructor();
        else if (CODRILLA.Role == 'admin')
            setupInstructor();
    };

    var setupLoggedOut = function () {
        CODRILLA = {
            Email: '',
            Role: '',
            Expires: serverTime(),
            LoggedIn: false
        };

        $('#loggedin').hide();
        $('#notloggedin').show();
        $('#tabs').tabs('option', 'disabled', [1, 2, 3, 4, 5, 6, 7, 8, 9]);
    };

    var setupStudent = function () {
        $('#tabs').tabs('option', 'disabled', [1, 2, 3, 4, 5, 6, 7, 8, 9]);
        refreshStudentSchedule(true);
    };

    var setupInstructor = function () {
        $('#tabs').tabs('option', 'disabled', [1, 2, 3, 4, 5, 6, 7, 8, 9]);
        $('#tab-instructor-problemeditor').data('problemTypeTag', 'python27stdin');
        refreshInstructorSchedule(true);
    };

    var refreshStudentSchedule = function (setTab) {
        $.getJSON('/student/courses', function (info) {
            var tobegradedcount = 0;
            var $div = $('#tab-student-schedule');
            $div
                .empty()
                .append('<h1>Courses and assignments</h1>');
            if (!info.Courses || info.Courses.length == 0) {
                $div.append('<p>You are not enrolled in any active courses</p>');
                return;
            }
            $.each(info.Courses, function (i, course) {
                var passed = 0, failed = 0, pending = 0;
                $('<h2 />').text(course.Name).appendTo($div);
                if (course.PastAssignments.length == 0 && course.OpenAssignments.length == 0 && course.FutureAssignments.length == 0) {
                    $div.append('<p>No assignments on the schedule</p>');
                    return;
                }
                var $table = $('<table />').appendTo($div);

                // past assignments
                $('<thead><tr class="collapse"><td colspan="4">Past Assignments</td></tr></thead>').appendTo($table);
                var $tbody = $('<tbody />').appendTo($table);
                $.each(course.PastAssignments, function (i, asst) {
                    $tbody.append(buildAssignmentRow(asst));

                    if (asst.ToBeGraded > 0) tobegradedcount++;
                    if (asst.Passed) passed++;
                    else if (!asst.Active && asst.ToBeGraded == 0) failed++;
                    else pending++;
                });
                if (course.PastAssignments.length == 0)
                    $('<tr><td colspan="4">No past assignments</td></tr>').appendTo($tbody);
                $('<tr class="blankrow"><td colspan="4">&nbsp;</td></tr>').appendTo($tbody);

                // open assignments
                $('<thead><tr class="collapse"><td colspan="4">Open Assignments</td></tr></thead>').appendTo($table);
                var $tbody = $('<tbody />').appendTo($table);
                $.each(course.OpenAssignments, function (i, asst) {
                    $tbody.append(buildAssignmentRow(asst));

                    if (asst.ToBeGraded > 0) tobegradedcount++;
                    if (asst.Passed) passed++;
                    else if (!asst.Active && asst.ToBeGraded == 0) failed++;
                    else pending++;
                });
                if (course.OpenAssignments.length == 0)
                    $('<tr><td colspan="4">No open assignments</td></tr>').appendTo($tbody);
                $('<tr class="blankrow"><td colspan="4">&nbsp;</td></tr>').appendTo($tbody);

                // future assignments
                $('<thead><tr class="collapse"><td colspan="4">Future Assignments</td></tr></thead>').appendTo($table);
                var $tbody = $('<tbody />').appendTo($table);
                $.each(course.FutureAssignments, function (i, asst) {
                    $tbody.append(buildAssignmentRow(asst));

                    if (asst.ToBeGraded > 0) tobegradedcount++;
                    if (asst.Passed) passed++;
                    else pending++;
                });
                if (course.FutureAssignments.length == 0)
                    $('<tr><td colspan="4">No future assignments</td></tr>').appendTo($tbody);
                $('<tr class="blankrow"><td colspan="4">&nbsp;</td></tr>').appendTo($tbody);

                // compile the grade report
                var total = passed + failed;
                var text = 'Total: ';
                text += passed + ' passed';
                if (total > 0)
                    text += ' (' + Math.round(100.0*passed/total) + '%)';
                text += ', ' + failed + ' failed ';
                if (total > 0)
                    text += ' (' + Math.round(100.0*failed/total) + '%)';
                text += ', ' + pending + ' still pending';

                $('<tfoot><tr><td colspan="4">' + text + '</td></tr></tfoot>').appendTo($table);
            });
            if (setTab)
                $('#tabs').tabs('enable', 1).tabs('option', 'active', 1);

            // schedule a refresh?
            if (tobegradedcount > 0) {
                window.setTimeout(refreshStudentSchedule, 2000);
            }
        });
    };
    $('.collapse').live('click', function () {
        $(this).parent('thead').next('tbody').toggle();
        return false;
    });
    $('.assignmentEditorLink').live('click', function (e) {
        if ($(e.target).is('a')) return true;
        var asst = $(this).data('asst');
        if (!asst) return;
        $('#tab-student-editor').data('assignmentID', asst.ID);
        refreshStudentEditor(true);
        return false;
    });

    var refreshStudentEditor = function (setTab) {
        var asstID = $('#tab-student-editor').data('assignmentID');
        if (!asstID) return;
        $.getJSON('/student/assignment/' + asstID, function (asst) {
            // are we waiting for a grading result?
            var waiting = asst.Assignment.ToBeGraded && asst.Assignment.ToBeGraded > 0;
            var readonly = waiting || !asst.Assignment.Active;

            var $div = $('#tab-student-editor');
            $div
                .empty();
            if (readonly)
                $div.append('<h1>Assignment Viewer</h1>');
            else
                $div.append('<h1>Assignment Editor</h1>');

            // display the general assignment info
            var $name = $('<b />').text('“' + asst.Assignment.Name + '”');
            $('<p />').append($name).appendTo($div);
            var $due = $('<p>Due </p>').appendTo($div);
            formatDate($due, asst.Assignment.Close);


            // prepare the editor
            var $editor = createEditor(asst.ProblemType.FieldList, asst.Data, 'student', readonly);
            $div.append($editor);

            if (asst.Assignment.Active) {
                // let them submit
                var $button = $('<button id="studentsubmitbutton">Submit solution</button>');
                $button.data('assignmentID', asstID);
                $button.appendTo($div);
            }

            if (setTab)
                $('#tabs').tabs('enable', 2).tabs('option', 'active', 2);

            $('.CodeMirror').each(function () { this.CodeMirror.refresh(); });

            // schedule a refresh?
            if (waiting)
                window.setTimeout(refreshStudentEditor, 2000);
        });
    };

    var refreshInstructorSchedule = function (setTab) {
        $.getJSON('/course/list', function (courseList) {
            var $div = $('#tab-instructor-schedule');
            $div
                .empty()
                .append('<h1>Courses and assignments</h1>');
            if (!courseList || courseList.length == 0) {
                $div.append('<p>You are not the instructor for any active courses</p>');
                return;
            }
            
            var $newProbTypeList = $('<select id="newproblemtypelist"></select>');
            $.getJSON('/problem/types', function (lst) {
                $.each(lst, function (i, elt) {
                    $('<option />').text(elt.Name).val(elt.Tag).appendTo($newProbTypeList);
                });
            });
            var $newProbButton = $('<button id="newproblembutton">Create new problem</button>');
            $newProbButton.data('problemID', undefined);
            $('<p />').append($newProbTypeList).append($newProbButton).appendTo($div);

            $.each(courseList, function (i, course) {
                $('<h3 />').text(course.Name).append(' (').append(course.Tag).append(')').appendTo($div);
                var $newAsstButton = $('<button class="newassignmentbutton">Create new assignment</button>');
                $newAsstButton.data('courseTag', course.Tag);
                $('<p />').append($newAsstButton).appendTo($div);

                if (course.OpenAssignments.length > 0) {
                    $('<h4>Open assignments</h4>').appendTo($div);
                    var $list = $('<ul />').appendTo($div);
                    $.each(course.OpenAssignments, function (i, asst) {
                        $list.append(buildAssignmentListItem(asst, true, true));
                    });
                }
                if (course.FutureAssignments.length > 0) {
                    $('<h4>Future assignments</h4>').appendTo($div);
                    var $list = $('<ul />').appendTo($div);
                    $.each(course.FutureAssignments, function (i, asst) {
                        $list.append(buildAssignmentListItem(asst));
                    });
                }
                if (course.ClosedAssignments.length > 0) {
                    $('<h4>Past assignments</h4>').appendTo($div);
                    var $list = $('<ul />').appendTo($div);
                    $.each(course.ClosedAssignments, function (i, asst) {
                        $list.append(buildAssignmentListItem(asst));
                    });
                }

                $('<h4>Update course membership</h4>').appendTo($div);
                var $ta = $('<textarea class="coursemembership"></textarea>').appendTo($div);
                $div.append('<br />');
                var $tb = $('<button class="coursemembershipbutton">Upload course list JSON</button>').appendTo($div);
                $tb.data('courseTag', course.Tag);

            });
            if (setTab)
                $('#tabs').tabs('enable', 5).tabs('option', 'active', 5);
        });
    };

    var refreshInstructorProblemEditor = function (setTab) {
        var problemTypeTag = $('#tab-instructor-problemeditor').data('problemTypeTag');
        var problemID = $('#tab-instructor-problemeditor').data('problemID');
        if (!problemTypeTag) return;
        $.getJSON('/problem/type/' + problemTypeTag, function (problemType) {
            var buildEditor = function (problemData) {
                var $div = $('#tab-instructor-problemeditor');
                $div
                    .empty()
                    .append('<h1>Problem Editor</h1>');

                $('<p />').text('Problem type: ' + problemType.Name).appendTo($div);

                var name = '';
                var tags = [];
                if (problemData) {
                    name = problemData.Name;
                    tags = problemData.Tags;
                    $('<h3 />').text('Editing existing problem #' + problemData.ID + ': ' + name)
                        .appendTo($div);
                    $('<p />').text('Tags: ' + tags.join(' '))
                        .appendTo($div);
                }

                var namefield = $('<input type="text" id="problemeditorname">').val(name);
                var tagsfield = $('<input type="text" id="problemeditortags">').val(tags.join(' '));
                $('<p>Problem name: </p>').append(namefield).appendTo($div);
                $('<p>Problem tags: </p>').append(tagsfield).appendTo($div);

                // prepare the editor
                var contents = {};
                if (problemData && problemData.Data) {
                    $.each(problemData.Data, function (key, value) {
                        contents[key] = value;
                    });
                }
                var $editor = createEditor(problemType.FieldList, contents, 'creator', false);
                $div.append($editor);

                // let them submit
                var $button = $('<button id="problemeditorsavebutton">Save problem</button>');
                $button.data('problemID', problemID);
                $button.appendTo($div);

                if (setTab)
                    $('#tabs').tabs('enable', 6).tabs('option', 'active', 6);

                $('.CodeMirror').each(function () { this.CodeMirror.refresh(); });
            };

            // if we are editing an existing problem, load it
            if (problemID)
                $.getJSON('/problem/get/' + problemID, buildEditor);
            else
                buildEditor();
        });
    };

    var refreshInstructorAssignmentEditor = function (setTab) {
        var courseTag = $('#tab-instructor-assignmenteditor').data('courseTag');
        if (!courseTag) return;
        var asst = $('#tab-instructor-assignmenteditor').data('asst');
        $.getJSON('/problem/tags', function (container) {
            var buildEditor = function (asstData) {
                var $div = $('#tab-instructor-assignmenteditor');
                $div
                    .empty()
                    .append('<h1>Assignment Editor</h1>');

                $('<h3 />').text('Course: ' + courseTag).appendTo($div);
                $('<h3>Start date (leave blank to start now)</h3>').appendTo($div);
                $('<input type="text" id="assignmentstartdate">')
                    .datepicker()
                    .appendTo($div);
                $('<h3>Due date</h3>').appendTo($div);
                var tonight = serverTime();
                tonight.setHours(23);
                tonight.setMinutes(59);
                tonight.setSeconds(59);
                tonight.setMilliseconds(0);
                $('<input type="text" id="assignmentduedate">')
                    .datepicker()
                    .appendTo($div)
                    .val(tonight);
                $('<h3>Pick a problem</h3>').appendTo($div);

                // sort problems by name
                container.Problems.sort(function (a, b) {
                    if (a.Name < b.Name) return -1;
                    if (a.Name > b.Name) return 1;
                    return 0;
                });

                $.each(container.Problems, function (i, problem) {
                    var $button = $('<input type="radio" name="problempicker">')
                        .val(problem.ID);
                    if (asst && asst.Problem == problem.ID)
                        $button.prop('selected', 'selected');
                    var name = $(' <b />').text(problem.Name);
                    var $editlink = $('<button class="editproblembutton">Edit</button>')
                        .data('problemTypeTag', problem.Type)
                        .data('problemID', problem.ID);
                    
                    $('<p />').appendTo($div)
                        .append($button)
                        .append(name)
                        .append(' (' + problem.Type + ')' +
                            ' Tags: ' + problem.Tags.join(' ') +
                            ' UsedBy: ' + problem.UsedBy.join(' ') +
                            ' ')
                        .append($editlink);
                });

                var $button = $('<button id="assignmenteditorsavebutton">Save assignment</button>');
                $button.appendTo($div);

                if (setTab)
                    $('#tabs').tabs('enable', 7).tabs('option', 'active', 7);
            };

            // if we are editing an existing assignment, load it
            buildEditor();
        });
    };

    var buildAssignmentListItem = function (asst, supresseditorlink, supressresultlink) {
        var $item = $('<li />');

        // color the item if appropriate
        var now = serverTime();
        var future = now < new Date(asst.Open);
        if (asst.Passed && asst.ToBeGraded == 0)
            $item.addClass('green');
        else if (!future && !asst.Active && asst.ToBeGraded == 0)
            $item.addClass('red');

        // form line (possibly with link) for assignment name
        var when = 'Open';
        if (!asst.Active) {
            when = future ? 'Future' : 'Past';
        }
        var text = when + ' assignment: “' + asst.Name + '”';
        var $p = $('<p />').append($('<b />').text(text)).appendTo($item);
        if (asst.Active && !supresseditorlink) {
            var $link = $('<a href="#" class="assignmenteditorlink">go to editor</a>')
                .data('assignmentID', asst.ID);
            $p.append(' (').append($link).append(')');
        }

        // add open date line if in future
        if (future) {
            var $open = $('<p />').text('Opens ');
            formatDate($open, asst.Open);
            $item.append($open);
        }

        // add due date line
        var $due = $('<p />').text('Due ');
        formatDate($due, asst.Close);
        $item.append($due);

        // report on submissions
        if (asst.ToBeGraded > 0) {
            $item.append($('<p />').text('You have ' +
                asst.ToBeGraded + ' submission' + (asst.ToBeGraded == 1 ? '' : 's') +
                ' waiting to be graded'));
        } else if (asst.Attempts == 0) {
            if (asst.Active)
                $item.append($('<p />').text('You have not submitted a solution yet'));
            else if (!future)
                $item.append($('<p />').text('FAILED: You did not submit a solution'));
        } else if (asst.Passed) {
            $item.append($('<p />').text('PASSED: Total of ' +
                asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's')));
        } else if (asst.Active) {
            $item.append($('<p />').text('You have made ' +
                asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's') +
                ' so far, but you have not passed yet'));
        } else {
            $item.append($('<p />').text('FAILED: Total of ' +
                asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's')));
        }
        if (asst.Attempts > 0 && !supressresultlink) {
            var $link = $('<a href="#" class="assignmentresultlink">See result of latest submission</a>');
            $link.data('assignmentID', asst.ID);
            $('<p />').append($link).appendTo($item);
        }

        // add a download link
        if (!future) {
            var $link = $('<a href="/student/download/' + asst.ID + '" target="_blank">Download this assignment</a>');
            $('<p />').append($link).appendTo($item);
        }
        return $item;
    };

    var buildAssignmentRow = function (asst) {
        var now = serverTime();
        var $row = $('<tr />').data('asst', asst);

        // color the row if appropriate
        if (asst.Passed && asst.ToBeGraded == 0)
            $row.addClass('green');
        else if (now > new Date(asst.Close) && asst.ToBeGraded == 0)
            $row.addClass('red');

        // name
        $('<td />').text(asst.Name).appendTo($row);

        // due/open date
        if (now < new Date(asst.Open)) {
            var $due = $('<td>Opens </td>').appendTo($row);
            formatDate($due, asst.Open);
        } else {
            var $due = $('<td>Due </td>').appendTo($row);
            formatDate($due, asst.Close);
        }

        // attempts
        var msg = '';
        if (asst.ToBeGraded > 0) {
            msg = asst.Attempts + ' attempts, ' + asst.ToBeGraded + ' ungraded';
        } else if (asst.Attempts == 0) {
            msg = 'nothing submitted';
        } else if (asst.Passed) {
            msg = 'passed after ' + asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's');
        } else {
            msg = asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's');
        }
        $('<td />').text(msg).appendTo($row);

        // download link
        if (now > new Date(asst.Open)) {
            var $link = $('<a href="/student/download/' + asst.ID + '" target="_blank">.zip</a>');
            $('<td />').append($link).appendTo($row);
        } else {
            $('<td>&nbsp;</td>').appendTo($row);
        }

        // editor/result link
        if (now > new Date(asst.Open))
            $row.addClass('assignmentEditorLink');

        return $row;
    };

    $('.assignmenteditorlink').live('click', function () {
        // load the assignment
        var asstID = $(this).data('assignmentID');
        $('#tab-student-editor').data('assignmentID', asstID);
        refreshStudentEditor(true);
        return false;
    });

    $('#studentsubmitbutton').live('click', function () {
        var $div = $('#tab-student-editor');
        var data = JSON.stringify(formToJson($div));
        var asstID = $(this).data('assignmentID');
        $('#tab-student-result').data('assignmentID', asstID);
        $.ajax({
            type: 'POST',
            url: '/student/submit/' + asstID,
            contentType: 'application/json; charset=utf-8',
            data: data,
            success: function (res, status, xhr) {
                $('#tab-student-editor').empty().data('assignmentID', asstID);
                $('#tabs').tabs('disable', 2).tabs('option', 'active', 1);
                refreshStudentSchedule();
                refreshStudentEditor(true);
            },
            error: function (res, status, xhr) {
                console.log('error submitting solution');
                console.log(res);
            }
        });
    });

    $('#problemeditorsavebutton').live('click', function () {
        var $div = $('#tab-instructor-problemeditor');
        var problemTypeTag = $('#tab-instructor-problemeditor').data('problemTypeTag');
        var name = $div.find('#problemeditorname').val();
        var tags = $div.find('#problemeditortags').val().split(/\s+/);
        if (tags.length > 0 && tags[0] == '')
            tags.shift();
        if (tags.length > 0 && tags[tags.length-1] == '')
            tags.pop();

        var data = formToJson($div);
        var problemID = $(this).data('problemID');
        var elt = {
            Name: name,
            Type: problemTypeTag,
            Tags: tags,
            Data: data
        };
        $.ajax({
            type: 'POST',
            url: '/problem/' + (problemID ? 'update/' + problemID : 'new'),
            contentType: 'application/json; charset=utf-8',
            data: JSON.stringify(elt),
            success: function (res, status, xhr) {
                $('#tab-instructor-problemeditor').empty();
                $('#tabs').tabs('disable', 6).tabs('option', 'active', 4);
                refreshInstructorSchedule(true);
            },
            error: function (res, status, xhr) {
                console.log('error saving problem');
                console.log(res);
            }
        });
    });

    $('#newproblembutton').live('click', function () {
        var $div = $('#tab-instructor-problemeditor');
        var problemTypeTag = $('#newproblemtypelist').val();
        $div.data('problemTypeTag', problemTypeTag);
        $div.data('problemID', undefined);
        refreshInstructorProblemEditor(true);
    });
    $('.editproblembutton').live('click', function () {
        var problemTypeTag = $(this).data('problemTypeTag');
        var problemID = $(this).data('problemID');
        var $div = $('#tab-instructor-problemeditor');
        $div.data('problemTypeTag', problemTypeTag);
        $div.data('problemID', problemID);
        refreshInstructorProblemEditor(true);
    });

    $('.newassignmentbutton').live('click', function () {
        var courseTag = $(this).data('courseTag');
        var $div = $('#tab-instructor-assignmenteditor');
        $div.data('courseTag', courseTag);
        refreshInstructorAssignmentEditor(true);
    });

    $('#assignmenteditorsavebutton').live('click', function () {
        var $div = $('#tab-instructor-assignmenteditor');
        var s = $('#assignmentstartdate').val();
        var start;
        if (!s) start = new Date(1970, 0, 1);
        else start = new Date(s);
        var end = new Date($('#assignmentduedate').val());
        var courseTag = $div.data('courseTag');
        var problemID = Number($div.find('input[name=problempicker]:checked').val());
        var elt = {
            Problem: problemID,
            Open: start,
            Close: end,
            ForCredit: true
        };
        var now = serverTime();
        if (end < now) {
            alert('Due date must be in the future');
            return;
        }
        if (!problemID) {
            alert('You must select a problem');
            return;
        }
        $.ajax({
            type: 'POST',
            url: '/course/newassignment/' + courseTag,
            contentType: 'application/json; charset=utf-8',
            data: JSON.stringify(elt),
            success: function (res, status, xhr) {
                $('#tab-instructor-assignmenteditor').empty();
                $('#tabs').tabs('disable', 7).tabs('option', 'active', 4);
                refreshInstructorSchedule(true);
            },
            error: function (res, status, xhr) {
                console.log('error saving assignment');
                console.log(res);
            }
        });
    });

    $('.coursemembershipbutton').live('click', function () {
        var courseTag = $(this).data('courseTag');
        if (!courseTag) return;
        var data = $(this).prev().prev('.coursemembership').val();
        $.ajax({
            type: 'POST',
            url: '/course/courselistupload/' + courseTag,
            contentType: 'application/json; charset=utf-8',
            data: data,
            success: function (res, status, xhr) {
                refreshInstructorSchedule(true);
            },
            error: function (res, status, xhr) {
                console.log('error submitting course listing data');
                console.log(res);
            }
        });
    });

    //
    //
    // Editors
    //
    //

    var createEditor = function (fieldList, contents, role, readonly) {
        var $main = $('<div />');

        // run through the list of fields
        $.each(fieldList, function (i, field) {
            var action;
            if (role == 'creator')
                action = field.Creator;
            else if (role == 'student')
                action = field.Student;
            else if (role == 'result')
                action = field.Result;
            else {
                console.log('createEditor failed with role =', role);
                return null;
            }
            if (action == 'nothing') return;

            if (field.List) {
                var $div = $('<div />');
                $div.data('field', field).data('role', role);

                if (action == 'edit' && field.Prompt)
                    $('<h2 />').text(field.Prompt).prependTo($div);
                else if (action == 'view' && field.Title)
                    $('<h2 />').text(field.Title).prependTo($div);
                else
                    $('<h2 />').text('Field prompt/description goes here').prependTo($div);

                if (action == 'edit') $div.addClass('editorlist');
                var value = contents[field.Name] || [field.Default];

                $.each(value, function (i, onevalue) {
                    var $elt = createEditorField(field, onevalue, role, readonly);
                    if ($elt) {
                        // delete the header and append an hrule
                        $elt.find('h2').first().remove();
                        if (action == 'edit')
                            $elt.prepend('<button class="close closeparentdiv">&times;</button>');
                        $elt.append('<hr>');
                        if ($elt.hasClass('editorelt'))
                            $elt.removeClass('editorelt').addClass('editorlistelt');
                        $elt.appendTo($div);
                    }
                });
                if (action == 'edit')
                    $('<button class="addeditorfield">Add</button>').appendTo($div);
                $div.appendTo($main);
            } else {
                var value = contents[field.Name];
                if (typeof value == 'undefined')
                    value = field.Default;
                var $div = createEditorField(field, value, role, readonly);
                if ($div) {
                    $div.data('field', field).data('role', role);
                    $div.appendTo($main);
                }
            }
        });

        return $main;
    };

    var createEditorField = function (field, value, role, readonly) {
        var action;
        if (role == 'creator')
            action = field.Creator;
        else if (role == 'student')
            action = field.Student;
        else if (role == 'result')
            action = field.Result;
        else {
            console.log('createEditorField failed with role =', role);
            return null;
        }

        if (action == 'nothing') return null;
        if (action == 'edit' && readonly) action = 'view';
        if (action == 'view' && (typeof(value) == 'undefined' || typeof(value) == 'string' && value.search(/^\s*$/) == 0)) return null;
      
        var $div = $('<div />');
        if (action == 'edit' && field.Prompt)
            $('<h2 />').text(field.Prompt).prependTo($div);
        else if (action == 'view' && field.Title)
            $('<h2 />').text(field.Title).prependTo($div);
        else
            $('<h2 />').text('Field prompt/description goes here').prependTo($div);

        if (field.Type == 'markdown' && action == 'edit') {
            // markdown editor
            var $editor = $('<textarea class="stringfield" />').val(value.replace(/\n*$/, ''));
            $div.append($editor);
            CodeMirror.fromTextArea($editor[0], {
                mode: 'markdown',
                lineNumbers: true,
                indentUnit: 4,
                change: function(cm) { $editor.val(cm.getValue()); }
            });
        } else if (field.Type == 'markdown' && action == 'view') {
            // markdown viewer
            var html = marked(value);
            $('<div />').html(html).appendTo($div);
        } else if (field.Type == 'python' && (action == 'edit' || action == 'view')) {
            // python editor/viewer
            var $editor = $('<textarea />').val(value.replace(/\n*$/, ''));
            if (action == 'edit')
                $editor.addClass('stringfield');
            $div.append($editor);
            CodeMirror.fromTextArea($editor[0], {
                mode: 'python',
                readOnly: action == 'view',
                lineNumbers: true,
                indentUnit: 4,
                change: function(cm) { $editor.val(cm.getValue()); }
            });
        } else if (field.Type == 'text' && (action == 'edit' || action == 'view')) {
            // text editor/viewer
            var $editor = $('<textarea />').val(value.replace(/\n*$/, ''));
            if (action == 'edit')
                $editor.addClass('stringfield');
            $div.append($editor);
            CodeMirror.fromTextArea($editor[0], {
                mode: 'text',
                readOnly: action == 'view',
                lineNumbers: true,
                indentUnit: 4,
                change: function(cm) { $editor.val(cm.getValue()); }
            });
        } else if (field.Type == 'int' && action == 'edit') {
            // int editor
            var $input = $('<input type="number" step="1" min="1" class="intfield">').val(value || 1);
            $div.append($input);
        } else if (field.Type == 'bool' && action == 'edit') {
            // bool editor
            var $input = $('<input type="checkbox" class="boolfield" value="true">').prop('checked', value == true);
            $div.append($input);
        } else if ((field.Type == 'int' || field.Type == 'string') && action == 'view') {
            // int / string viewer
            $('<p />').text(value).appendTo($div);
        } else if (field.Type == 'bool' && action == 'view') {
            // bool viewer
            $('<p />').text(typeof value == 'boolean' ? (value ? 'Yes' : 'No') : 'Unknown').appendTo($div);
        } else if (field.Type == 'string' && action == 'edit') {
            // string editor
            var $input = $('<input type="text" class="stringfield">').val(value.replace(/\n*$/, ''));
            $div.append($input);
        } else {
            console.log('createEditorField: not implemented for Type =', field.Type, 'and action =', action);
            return null;
        }
        if (action == 'edit')
            $div.addClass('editorelt');

        return $div;
    };

    $('.closeparentdiv').live('click', function () {
        $(this).closest('div').remove();
        return false;
    });
    $('.addeditorfield').live('click', function () {
        var $div = $(this).closest('div.editorlist');
        var field = $div.data('field');
        var role = $div.data('role');
        var $elt = createEditorField(field, field.Default, role, false);
        if ($elt.hasClass('editorelt'))
            $elt.removeClass('editorelt').addClass('editorlistelt');
        $elt.find('h2').first().remove();
        $elt.prepend('<button class="close closeparentdiv">&times;</button>');
        $elt.append('<hr>');
        $elt.insertBefore(this);
        $('.CodeMirror').each(function () { this.CodeMirror.refresh(); });
        return false;
    });

    formToJsonGetElt = function (field, $div) {
        var result;
        $div.find('.stringfield').each(function () {
            result = String(this.value);
        });
        $div.find('.intfield').each(function () {
            result = Number(this.value);
        });
        $div.find('.boolfield').each(function () {
            result = Boolean(this.value);
        });
        return result;
    };

    formToJson = function ($root) {
        var o = {};

        $root.find('.CodeMirror').each(function(i, elt) {
            elt.CodeMirror.save();
        });

        // grab single elements
        $root.find('div.editorelt').each(function () {
            var $div = $(this);
            var field = $div.data('field');
            o[field.Name] = formToJsonGetElt(field, $div);
        });

        // gather lists
        $root.find('div.editorlist').each(function () {
            var $div = $(this);
            var field = $div.data('field');
            var list = [];
            $(this).find('div.editorlistelt').each(function () {
                var elt = formToJsonGetElt(field, $(this));
                if (typeof elt == 'number' || elt)
                    list.push(elt);
            });
            o[field.Name] = list;
        });
        return o;
    };

    var until = function (when) {
        var now = serverTime();
        var seconds = Math.floor((when.getTime() - now.getTime()) / 1000.0);
        var sign = (seconds < 0 ? ' ago' : ' from now');
        seconds = Math.abs(seconds);
        var d = Math.floor(seconds / (24*3600)); seconds -= d * 24*3600;
        var h = Math.floor(seconds / 3600); seconds -= h * 3600;
        var m = Math.floor(seconds / 60); seconds -= m * 60;
        var s = seconds;

        if (d >= 7)
            return d + 'd' + sign;
        if (d >= 1)
            return d + 'd' + h + 'h' + sign;
        if (h >= 6)
            return h + 'h' + sign;
        if (h >= 1)
            return h + 'h' + m + 'm' + sign;
        if (m >= 10)
            return m + 'm' + sign;
        if (m >= 1)
            return m + 'm' + s + 's' + sign;
        return s + 's' + sign;
    };

    var months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    var daysOfWeek = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
    var pad = function (n) {
        if (n < 10) return '0' + n;
        else return String(n);
    };
    var formatDate = function ($container, utc) {
        var when = new Date(utc);
        var now = serverTime();
        var stamp = daysOfWeek[when.getDay()] + ', ' + months[when.getMonth()] + ' ' + when.getDate();
        if (when.getFullYear() != now.getFullYear())
            stamp += ', ' + when.getFullYear();
        stamp += ' ' + when.getHours() + ':' + pad(when.getMinutes());
        $container.append(stamp + ' (');
        var $countdown = $('<span class="countdown">' + until(when) + '</span>');
        $countdown.data('deadline', when);
        $container.append($countdown).append(')');
    };
    window.setInterval(function () {
        $('.countdown').each(function () {
            var when = $(this).data('deadline');
            if (!when) return;
            $(this).text(until(when));
        });
    }, 1000);

    $('#tabs').tabs();
    if (CODRILLA.LoggedIn) {
        setupLoggedIn();
    } else {
        setupLoggedOut();
    }
    var pre = new Date();
    $.getJSON('/auth/time', function (server) {
        // figure out roughly how far off our clock is from the server
        var now = new Date();
        var latency = Math.round((now - pre) * 0.5);
        var really = new Date(server);
        skew = (really - now) + latency;
    });
});
