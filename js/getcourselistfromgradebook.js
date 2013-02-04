(function () {
	var lst = [];
	$('#gradebook_students_grid .slick-viewport .grid-canvas .slick-row').each(function () {
		var name = $(this).find('.student-name').text();
		var email = $(this).find('.secondary_identifier_cell').text();
		lst.push([name, email]);
	});
	return JSON.stringify(lst);
})();
