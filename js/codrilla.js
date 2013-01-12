jQuery(function ($) {
    var getCookies = function () {
        CODRILLA = {
              Email: '',
              Role: '',
              Expires: 0,
              LoggedIn: false
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
                },
                error: function(res, status, xhr) {
                    console.log('logout failure');
                    console.log(res);
                }
            });
        } 
    });
    $('#persona-login-button').click(function () {
        navigator.id.request();
        return false;
    });
    $('#logout-button').click(function () {
        navigator.id.logout();
        return false;
    });

    var setupNewProblem = function () {
        $('#newproblemspace').empty();

        // get the problem type
        var kind = $('#problemtype').val();
        var desc;
        $.each(CODRILLA.ProblemTypes, function (i, elt) {
            if (elt.Tag == kind)
                desc = elt.Description;
        });
        if (!desc)
            return;

        // fill in the form
        var role = CODRILLA.Role;
        role = 'instructor';
        var content = {};
        $.each(desc, function (i, elt) {
            var field = createProblemField(elt, content, role);
            if (field)
                $('#newproblemspace').append(field);
        });
    };

    var createProblemField = function (desc, content, role) {
        var action;
        if (role == 'instructor')
            action = desc.Instructor;
        else if (role == 'student')
            action == desc.Student;
        else
            return null;

        if (action == 'nothing') return null;
      
        // markdown editor
        if (desc.Type == 'markdown' && action == 'edit') {
            var $editor = $('<textarea name="' + desc.Name + '" class="stringfield" />');
            var $div = $('<div />').append($editor);
            CodeMirror.fromTextArea($editor[0], {
                mode: 'markdown',
                lineNumbers: true,
                indentUnit: 4,
                change: function(cm) { $editor.val(cm.getValue()); }
            });

            if (desc.Prompt)
                $('<h2 />').text(desc.Prompt).prependTo($div);
            return $div;
        }

        // markdown viewer
        if (desc.Type == 'markdown' && action == 'view') {
            var md = content[desc.Name] || '*Warning! ' + desc.Name + ' missing*';
            var html = marked(md);
            var $div = $('<div />').html(html);
            if (desc.Title)
                $('<h2 />'.text(desc.Title).prependTo($div));
            return $div;
        }

        // python editor/viewer
        if (desc.Type == 'python' && (action == 'edit' || action == 'view')) {
            var $editor = $('<textarea name="' + desc.Name + '" class="stringfield" />');
            var $div = $('<div />').append($editor);
            CodeMirror.fromTextArea($editor[0], {
                mode: 'python',
                readOnly: action == 'view',
                lineNumbers: true,
                indentUnit: 4,
                change: function(cm) { $editor.val(cm.getValue()); }
            });

            if (desc.Prompt)
                $('<h2 />').text(desc.Prompt).prependTo($div);
            return $div;
        }

        // int editor
        if (desc.Type == 'int' && action == 'edit') {
            var $input = $('<input type="number" step="1" min="1" name="' + desc.Name + '" value="' + desc.Default + '" class="intfield">');
            var $div = $('<div />').append($input);
            if (desc.Prompt)
                $('<h2 />').text(desc.Prompt).prependTo($div);
            return $div;
        }

        // int viewer
        if (desc.Type == 'int' && action == 'view') {
            var $div = $('<div />');
            var value = content[desc.Name] || 0;
            if (desc.Title)
                $('<h2 />').text(desc.Title + ': ' + value);
            else
                $('<h2 />').text(value);
            return $div;
        }

        // textfilelist editor
        if (desc.Type == 'textfilelist' && action == 'edit') {
            var $div = $('<div />')
                .append('<input type="hidden" class="listnameholder" value="' + desc.Name + '">')
                .append('<button class="btn btn-primary addtexteditor">Add</button>');
            if (desc.Prompt)
                $('<h2 />').text(desc.Prompt).prependTo($div);
            return $div;
        }

        return null;
    };

    $('.closeparentdiv').live('click', function () {
        $(this).closest('div').remove();
        renumber();
        return false;
    });
    $('.addtexteditor').live('click', function() {
        var name = $(this).closest('div').find('.listnameholder').val();
        var $editor = $('<textarea name="' + name + '" class="stringlistfield" />');
        var $inner = $('<div />')
            .append('<hr />')
            .append($editor)
            .append('<button class="close closeparentdiv">&times;</button>');
        $(this).closest('div').append($inner);

        CodeMirror.fromTextArea($editor[0], {
            mode: 'text',
            lineNumbers: true,
            change: function(cm) { $editor.val(cm.getValue()); }
        });

        return false;
    });

    formToJson = function ($root) {
        var o = {};

        $root.find('.CodeMirror').each(function(i, elt) {
            elt.CodeMirror.save();
        });
        $root.find('.stringfield').each(function () {
            if (!this.name) {
                console.log('Warning! formToJson found a stringfield without a name');
                return;
            }
            o[this.name] = String(this.value);
        });
        $root.find('.intfield').each(function () {
            if (!this.name) {
                console.log('warning! formtojson found an intfield without a name');
                return;
            }
            o[this.name] = Number(this.value);
        });
        $root.find('.listnameholder').each(function () {
            var name = this.value;
            if (!name) {
                console.log('warning! formtojson found a listnameholder without a name');
                return;
            }
            o[name] = [];

            var $div = $(this).closest('div');
            $div.find('.stringlistfield').each(function (i, elt) {
                o[name].push(String(this.value));
            });
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

        // set up the nav bar to only show the account tab
        $('ul.nav > li').first().tab('show').siblings().hide();
        $('ul.nav > li > a').first().click();
    };

    var setupLoggedIn = function () {
        $('#loggedin').show();
        $('#loggedin-as').text(CODRILLA.Email);
        $('#notloggedin').hide();

        // hide everything except the account tab
        $('ul.nav > li').first().tab('show').siblings().hide();

        if (CODRILLA.Role == 'student')
            setupStudent();
        else if (CODRILLA.Role == 'instructor')
            setupInstructor();
        else if (CODRILLA.Role == 'admin')
            setupInstructor();
    };

    var setupStudent = function () {
        $('a[data-toggle="tab"][href="#tab-overview"]').tab('show').parent().show();
        $('a[data-toggle="tab"][href="#tab-assignments"]').parent().show();
    };

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

    var setupAdmin = function () {
        $('a[data-toggle="tab"][href="#tab-create-course"]').parent().show();
    };

    if (CODRILLA.LoggedIn) {
        setupLoggedIn();
    } else {
        setupLoggedOut();
    }
});
