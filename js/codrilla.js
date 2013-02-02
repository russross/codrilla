    jQuery(function ($) {
        var getCookies = function () {
            CODRILLA = {
                  Email: '',
                  Role: '',
                  Expires: 0,
                  LoggedIn: false,
                  LoginMethod: 'google'
            };
            var n = Number($.cookie('codrilla-expires'));
            CODRILLA.Expires = new Date(n * 1000);
            var now = new Date();
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
                '&redirect_uri=http://localhost:8080/auth/login/google' +
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

    var createEditor = function (fieldList, contents, role) {
        var $main = $('<div />');

        // run through the list of fields
        $.each(fieldList, function (i, field) {
            var action;
            if (role == 'instructor')
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
                    var $elt = createEditorField(field, onevalue, role);
                    if ($elt) {
                        // delete the header and append an hrule
                        $elt.find('h2').first().remove();
                        if (action == 'edit')
                            $elt.append('<button class="close closeparentdiv">&times;</button>');
                        $elt.append('<hr>');
                        if ($elt.hasClass('editorelt'))
                            $elt.removeClass('editorelt').addClass('editorlistelt');
                        $elt.appendTo($div);
                    }
                });
                if (action == 'edit')
                    $('<button class="btn btn-primary addeditorfield">Add</button>').appendTo($div);
                $div.appendTo($main);
            } else {
                var value = contents[field.Name];
                if (typeof value == 'undefined')
                    value = field.Default;
                var $div = createEditorField(field, value, role);
                if ($div) {
                    $div.data('field', field).data('role', role);
                    $div.appendTo($main);
                }
            }
        });

        return $main;
    };

    var createEditorField = function (field, value, role) {
        var action;
        if (role == 'instructor')
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
      
        var $div = $('<div />');
        if (action == 'edit' && field.Prompt)
            $('<h2 />').text(field.Prompt).prependTo($div);
        else if (action == 'view' && field.Title)
            $('<h2 />').text(field.Title).prependTo($div);
        else
            $('<h2 />').text('Field prompt/description goes here').prependTo($div);

        if (field.Type == 'markdown' && action == 'edit') {
            // markdown editor
            var $editor = $('<textarea class="stringfield" />').val(value);
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
            var $editor = $('<textarea />').val(value);
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
            var $editor = $('<textarea />').val(value);
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
            var $input = $('<input type="text" class="stringfield">').val(value);
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
        var $elt = createEditorField(field, field.Default, role);
        if ($elt.hasClass('editorelt'))
            $elt.removeClass('editorelt').addClass('editorlistelt');
        $elt.find('h2').first().remove();
        $elt.append('<button class="close closeparentdiv">&times;</button>');
        $elt.append('<hr>');
        $elt.insertBefore(this);
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
        return JSON.stringify(o);
    };

    var setupLoggedOut = function () {
      CODRILLA = {
            Email: '',
            Role: '',
            Expires: new Date(),
            LoggedIn: false
        };

        $('#loggedin').hide();
        $('#notloggedin').show();
        $('#tabs').tabs('option', 'disabled', [1, 2, 3, 4]);
    };

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

    var setupStudent = function () {
        $('#tabs').tabs('option', 'disabled', [1, 2, 3, 4]);
        refreshStudentSchedule(true);
    };

    var refreshStudentSchedule = function (setTab) {
        $.getJSON('/student/courses', function (info) {
            var tobegradedcount = 0;
            var $div = $('#tab-student-schedule');
            $div
                .empty()
                .append('<h2>Courses and assignments</h2>');
            if (!info.Courses || info.Courses.length == 0) {
                $div.append('<p>You are not enrolled in any active courses</p>');
                return;
            }
            info.Courses.sort(function (a, b) {
                if (a.Name < b.Name) return -1;
                else if (a.Name > b.Name) return 1;
                else return 0;
            });
            $.each(info.Courses, function (i, course) {
                if (!course.OpenAssignments) course.OpenAssignments = [];
                var $link = $('<a href="#" class="courselink" />');
                $link.data('course', course);
                $link.text('grade report');
                $('<h3 />').text(course.Name).append(' (').append($link).append(')').appendTo($div);
                if (course.OpenAssignments.length == 0 && !course.NextAssignment) {
                    $div.append('<p>No future assignments on the schedule yet</p>');
                    return;
                }
                var $list = $('<ul />').appendTo($div);
                course.OpenAssignments.sort(function (a, b) {
                    if (a.Close < b.Close) return -1;
                    else if (a.Close > b.Close) return 1;
                    else return 0;
                });
                $.each(course.OpenAssignments, function (i, asst) {
                    if (asst.ToBeGraded > 0)
                        tobegradedcount++;
                    $list.append(buildAssignmentListItem(asst));
                });
                if (course.NextAssignment) {
                    var $item = $('<li />').appendTo($list);
                    $item.append($('<p />').append($('<b />').text('Next upcoming assignment: “' +
                        course.NextAssignment.Name + '”')));
                    var $opens = $('<p />').text('Opens ');
                    formatDate($opens, course.NextAssignment.Open);
                    var $due = $('<p />').text('Due ');
                    formatDate($due, course.NextAssignment.Close);
                    $item.append($due);
                }
            });
            if (setTab)
                $('#tabs').tabs('enable', 1).tabs('option', 'active', 1);

            // schedule a refresh?
            if (tobegradedcount > 0) {
                window.setTimeout(refreshStudentSchedule, 2000);
            }
        });
    };
    $('.courselink').live('click', function () {
        // load a grade report for this course
        var course = $(this).data('course');
        $('#tab-student-grades').data('course', course);
        refreshStudentGrade(true);
        return false;
    });

    var refreshStudentResult = function (setTab) {
        var asstID = $('#tab-student-result').data('assignmentID');
        if (!asstID) return;
        $.getJSON('/student/result/' + asstID + '/-1', function (asst) {
            var $div = $('#tab-student-result')
            $div
                .empty()
                .append('<h2>Result Viewer</h2>');

            // display the general assignment info
            var $list = $('<ul />').appendTo($div);
            $list.append(buildAssignmentListItem(asst.Assignment, false, true));

            // prepare the editor
            var contents = {};
            $.each(asst.ProblemData || {}, function (key, value) {
                contents[key] = value;
            });
            $.each(asst.Attempt || {}, function (key, value) {
                contents[key] = value;
            });
            $.each(asst.ResultData || {}, function (key, value) {
                contents[key] = value;
            });
            var $editor = createEditor(asst.ProblemType.FieldList, contents, 'result');
            $div.append($editor);

            if (setTab)
                $('#tabs').tabs('enable', 3).tabs('option', 'active', 3);

            $('.CodeMirror').each(function () { this.CodeMirror.refresh(); });

            // schedule a refresh?
            if (!asst.ResultData || asst.ResultData.length == 0) {
                window.setTimeout(refreshStudentResult, 2000);
            }
        });
    };

    var buildAssignmentListItem = function (asst, supresseditorlink, supressresultlink) {
        var $item = $('<li />');

        // color the item if appropriate
        if (asst.Passed && asst.ToBeGraded == 0)
            $item.addClass('green');
        else if (!asst.Active && asst.ToBeGraded == 0)
            $item.addClass('red');

        // form line (possibly with link) for assignment name
        var text = (asst.Active ? 'Open' : 'Past') + ' assignment: “' + asst.Name + '”';
        var $p = $('<p />').append($('<b />').text(text)).appendTo($item);
        if (asst.Active && !supresseditorlink) {
            var $link = $('<a href="#" class="asstlink">go to editor</a>')
                .data('assignmentID', asst.ID);
            $p.append(' (').append($link).append(')');
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
            else
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
            var $link = $('<a href="#" class="latestattemptlink">See result of latest submission</a>');
            $link.data('assignmentID', asst.ID);
            $('<p />').append($link).appendTo($item);
        }
        return $item;
    };

    $('.latestattemptlink').live('click', function () {
        var asstID = $(this).data('assignmentID');
        $('#tab-student-result').data('assignmentID', asstID);
        refreshStudentResult(true);
        return false;
    });

    var until = function (when) {
        var now = new Date();
        var seconds = Math.floor((when.getTime() - now.getTime()) / 1000.0);
        var sign = (seconds < 0 ? '-' : '');
        seconds = Math.abs(seconds);
        var d = Math.floor(seconds / (24*3600)); seconds -= d * 24*3600;
        var h = Math.floor(seconds / 3600); seconds -= h * 3600;
        var m = Math.floor(seconds / 60); seconds -= m * 60;
        var s = seconds;

        if (d >= 7)
            return sign + d + 'd';
        if (d >= 1)
            return sign + d + 'd' + h + 'h';
        if (h >= 6)
            return sign + h + 'h';
        if (h >= 1)
            return sign + h + 'h' + m + 'm';
        if (m >= 10)
            return sign + m + 'm';
        if (m >= 1)
            return sign + m + 'm' + s + 's';
        return sign + s + 's';
    };

    var refreshStudentGrade = function (setTab) {
        var course = $('#tab-student-grades').data('course');
        if (!course) return;
        $.getJSON('/student/grades/' + course.Tag, function (grades) {
            var $div = $('#tab-student-grades').empty();
            $('<h2 />').text('Grade report for: ' + course.Name).appendTo($div);
            var $list = $('<ul />').appendTo($div);
            var passed = 0, failed = 0, pending = 0, tobegradedcount = 0;
            grades.sort(function (a, b) {
                if (a.Close < b.Close) return -1;
                else if (a.Close > b.Close) return 1;
                else return 0;
            });
            $.each(grades, function (i, asst) {
                if (!asst.ForCredit) return;

                if (asst.ToBeGraded > 0)
                    tobegradedcount++;

                if (asst.Passed) passed++;
                else if (!asst.Active && asst.ToBeGraded == 0) failed++;
                else pending++;

                $list.append(buildAssignmentListItem(asst));
            });

            var total = passed + failed;
            var text = 'Total: ';
            text += passed + ' passed';
            if (total > 0)
                text += ' (' + 100.0*passed/total + '%)';
            text += ', ' + failed + ' failed ';
            if (total > 0)
                text += ' (' + 100.0*failed/total + '%)';
            text += ', ' + pending + ' still pending';
            $('<p />').append($('<b />').text(text)).appendTo($div);

            if (setTab)
                $('#tabs').tabs('enable', 4).tabs('option', 'active', 4);

            // schedule a refresh?
            if (tobegradedcount > 0) {
                window.setTimeout(refreshStudentGrade, 10000);
            }
        });
    };

    var months = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
    var daysOfWeek = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
    var pad = function (n) {
        if (n < 10) return '0' + n;
        else return String(n);
    };
    var formatDate = function ($container, unix) {
        var when = new Date(1000 * unix);
        var now = new Date();
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
    }, 5000);

    $('.asstlink').live('click', function () {
        // load the assignment
        var asstID = $(this).data('assignmentID');
        $.getJSON('/student/assignment/' + asstID, function (asst) {
            var $div = $('#tab-student-assignment');
            $div
                .empty()
                .append('<h2>Assignment Editor</h2>');

            // display the general assignment info
            var $list = $('<ul />').appendTo($div);
            $list.append(buildAssignmentListItem(asst.Assignment, true, false));

            // prepare the editor
            var contents = {};
            $.each(asst.ProblemData || {}, function (key, value) {
                contents[key] = value;
            });
            $.each(asst.Attempt || {}, function (key, value) {
                contents[key] = value;
            });
            var $editor = createEditor(asst.ProblemType.FieldList, contents, 'student');
            $div.append($editor);

            // let them submit
            var $button = $('<button id="studentsubmit">Submit solution</button>');
            $button.data('assignmentID', asstID);
            $button.appendTo($div);

            $('#tabs').tabs('enable', 2).tabs('option', 'active', 2);

            $('.CodeMirror').each(function () { this.CodeMirror.refresh(); });
        });

        return false;
    });
    $('#studentsubmit').live('click', function () {
        var $div = $('#tab-student-assignment');
        var data = formToJson($div);
        var asstID = $(this).data('assignmentID');
        $('#tab-student-result').data('assignmentID', asstID);
        $.ajax({
            type: 'POST',
            url: '/student/submit/' + asstID,
            contentType : 'application/json; charset=utf-8',
            data: data,
            success: function (res, status, xhr) {
                $('#tab-student-assignment').empty();
                $('#tabs').tabs('disable', 2).tabs('option', 'active', 1);
                refreshStudentSchedule();
                refreshStudentGrade();
                refreshStudentResult(true);
            },
            error: function (res, status, xhr) {
                console.log('error submitting solution');
                console.log(res);
            }
        });
            
    });

    var setupInstructor = function () {
        $('a[data-toggle="tab"][href="#tab-overview"]').tab('show').parent().show();
        $('a[data-toggle="tab"][href="#tab-assignments"]').parent().show();
        $('a[data-toggle="tab"][href="#tab-setup"]').parent().show();
        $('a[data-toggle="tab"][href="#tab-create-problem"]').parent().show();

        // get the list of problem types for problem creation
        $.getJSON('/problem/listtypes', function (types) {
            // types is a list of:
            //   Name
            //   Tag
            //   Description
            CODRILLA.ProblemTypes = types;

            // fill in the select box on the create problem tab
            $('#problemtype').empty();
            $.each(types, function (i, elt) {
                $('<option value="' + elt.Tag + '" />').text(elt.Name).appendTo('#problemtype');
            });
            //$('#problemtype').change();
            setupNewProblem();
        });
    };

    $('#tabs').tabs();
    if (CODRILLA.LoggedIn) {
        setupLoggedIn();
    } else {
        setupLoggedOut();
    }
});
