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
						success: function(res, status, xhr) {
							getCookies();
							setupLoggedIn();
						},
						error: function(res, status, xhr) {
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

    var setupNewProblem = function () {
        $('#newproblemspace').empty();

        // get the problem type
        var kind = $('#problemtype').val();
        var fieldlist;
        $.each(CODRILLA.ProblemTypes, function (i, elt) {
            if (elt.Tag == kind)
                fieldlist = elt.Description;
        });
        if (!fieldlist)
            return;

        // fill in the form
        var role = CODRILLA.Role;
        role = 'instructor';
        var content = {};
        var $editor = createEditor(fieldlist, content, role);
        $editor.appendTo('#newproblemspace');
    };

    var createEditor = function (fieldList, contents, role) {
        var $main = $('<div />');

        // run through the list of fields
        $.each(fieldList, function (i, field) {
            var action;
            if (role == 'instructor')
                action = field.Editor;
            else if (role == 'student')
                action = field.Student;
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
                var value = contents[field.Name] || field.Default;
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
            action = field.Editor;
        else if (role == 'student')
            action = field.Student;
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
            var $editor = $('<textarea />');
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
            var $editor = $('<textarea />');
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
        } else if ((field.Type == 'int' || field.Type == 'string') && action == 'view') {
            // int / string viewer
            $('<p />').text(value).appendTo($div);
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
		$('#tabs').tabs('option', 'disabled', [1, 2, 3]);
		$.getJSON('/student/courses', function (info) {
			var $div = $('#tab-student-schedule');
			$div
				.empty()
				.append('<h2>Courses and assignments</h2>');
			if (!info.Courses || info.Courses.length == 0) {
				$div.append('<p>You are not enrolled in any active courses</p>');
				return;
			}
			$.each(info.Courses, function (i, course) {
				if (!course.OpenAssignments) course.OpenAssignments = [];
				var $link = $('<a href="#" class="courselink" />');
				$link.data('course', course);
				$link.text(course.Name);
				$('<h3 />').append($link).appendTo($div);
				if (course.OpenAssignments.length == 0 && !course.NextAssignment) {
					$div.append('<p>No future assignments on the schedule yet</p>');
					return;
				}
				var $list = $('<ul />').appendTo($div);
				$.each(course.OpenAssignments, function (i, asst) {
					var $item = $('<li />');
					var text = 'Open assignment: “' + asst.Name + '”';
					var $link = $('<a href="#" class="asstlink" />');
					$link.data('assignmentID', asst.ID);
					$link.text(text);
					$('<p />').append($('<b />').append($link)).appendTo($item);
					$item.append($('<p />').text('Due ' + new Date(1000 * asst.Close)));
					if (asst.ToBeGraded > 0) {
						$item.append($('<p />').text('You have ' +
							asst.ToBeGraded + ' submission' + (asst.ToBeGraded == 1 ? '' : 's') +
							' waiting to be graded'));
					} else if (asst.Attempts == 0) {
						$item.append($('<p />').text('You have not submitted a solution yet'));
					} else if (asst.Passed) {
						$item.append($('<p />').text('PASSED (total of ' +
							asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's') + ')'));
					} else {
						$item.append($('<p />').text('You have made ' +
							asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's') +
							' so far, but you have not passed yet'));
					}
					if (!asst.ForCredit) {
						$item.append($('<p />').text('(note: this assignment does not count ' +
							'toward your grade)'));
					}
					$item.appendTo($list);
				});
				if (course.NextAssignment) {
					var $item = $('<li />').appendTo($list);
					$item.append($('<p />').append($('<b />').text('Next upcoming assignment: “' +
						course.NextAssignment.Name + '”')));
					$item.append($('<p />').text('Opens ' +
						new Date(1000 * course.NextAssignment.Open)));
					$item.append($('<p />').text('Due ' +
						new Date(1000 * course.NextAssignment.Close)));
				}
			});
			$('#tabs').tabs('enable', 1).tabs('option', 'active', 1);
		});
    };
	$('.courselink').live('click', function () {
		// load a grade report for this course
		var course = $(this).data('course');
		$.getJSON('/student/grades/' + course.Tag, function (grades) {
			var now = new Date().getTime() / 1000.0;
			var $div = $('#tab-student-grades').empty();
			$('<h2 />').text('Grade report for: ' + course.Name).appendTo($div);
			var $list = $('<ul />').appendTo($div);
			var passed = 0, failed = 0, pending = 0;
			$.each(grades, function (i, asst) {
				if (!asst.ForCredit)
					return;
				var $item = $('<li />').appendTo($list);
				if (asst.Passed) {
					$item.addClass('green');
					passed++;
				} else if (!asst.Active) {
					$item.addClass('red');
					failed++;
				} else {
					pending++;
				}
				var text = (asst.Active ? 'Open' : 'Past') +
					' assignment: “' + asst.Name + '”';
				$('<p />').append($('<b />').text(text)).appendTo($item);

				$item.append($('<p />').text('Due ' + new Date(1000 * asst.Close)));
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
						asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's') + ')'));
				} else {
					if (asst.Active) {
						$item.append($('<p />').text('You have made ' +
							asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's') +
							' so far, but you have not passed yet'));
					} else {
						$item.append($('<p />').text('FAILED: Total of ' +
							asst.Attempts + ' attempt' + (asst.Attempts == 1 ? '' : 's') + ')'));
					}
				}
			});

			var total = passed + failed;
			var text = 'Total: ';
			text += passed + ' passed';
			if (total > 0)
				text += ' (' + 100.0*passed/total + '%)';
			text += ', ' + failed + ' failed, ';
			if (total > 0)
				text += ' (' + 100.0*failed/total + '%)';
			text += ', ' + pending + ' still open';
			$('<p />').append($('<b />').text(text)).appendTo($div);

			$('#tabs').tabs('enable', 3).tabs('option', 'active', 3);
		});

		return false;
	});
	$('.asstlink').live('click', function () {
		// load the assignment
		var asstID = $(this).data('assignmentID');
		$.getJSON('/student/assignment/' + asstID, function (asst) {
			var $div = $('#tab-student-assignment').empty();
			$('<h2 />').text('Assignment ' + asst.Assignment.Name).appendTo($div);
			var contents = {};
			$.each(asst.ProblemData || {}, function (key, value) {
				contents[key] = value;
			});
			$.each(asst.Attempt || {}, function (key, value) {
				contents[key] = value;
			});
			var $editor = createEditor(asst.ProblemType.FieldList, contents, 'student');
			$div.append($editor);

			$('#tabs').tabs('enable', 2).tabs('option', 'active', 2);
		});

		return false;
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
