jQuery(function ($) {
    $('#logout').hide();

    // login handling
    navigator.id.watch({
        loggedInUser: null,
        onlogin: function(assertion) {
            $('#login').hide();
            $('#user').text('logging in...');
            $.ajax({
                type: 'POST',
                url: '/auth/login/browserid',
                dataType: 'json',
                data: { Assertion: assertion },
                success: function(res, status, xhr) {
                    $('#user').text(res.Email);
                    $('#logout').show();
                },
                error: function(res, status, xhr) {
                    $('#user').empty();
                    $('#login').show();
                    console.log('login failure');
                    console.log(res);
                }
            });
        },
        onlogout: function() {
            $('#logout').hide();
            $.ajax({
                type: 'POST',
                url: '/auth/logout',
                success: function(res, status, xhr) {
                    $('#user').empty();
                    $('#login').show();
                    $('#logout').hide();
                },
                error: function(res, status, xhr) {
                    console.log('logout failure');
                    console.log(res);
                }
            });
        } 
    });
    $('#login').click(function () {
        navigator.id.request();
        return false;
    });
    $('#logout').click(function () {
        navigator.id.logout();
        return false;
    });
});
