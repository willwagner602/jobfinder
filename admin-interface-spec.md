Several areas of improvement:

Admin interface
	The admin interface is made up of 3 pages:
	- User management
		- list of users
			- per user disable/enable button. Disable removes a user's ability to login, but does not allow them to signup for a new account with the same email
			- click name for individual user data page (basic page layout shows all data for now)
			- delete button for user should just tombstone them - tombstoned users should not be considered during the new user email signup process, or be allowed to login.
			- Full site enable/disable signups button - when set to disable, this must block all new user signups. It can either make the page inaccessible, or respond negatively to all POST requests from it
	- Job management
		- job history: start, end, results displayed

Overall look & feel, UI
	- Modern look & feel
	- Page sections should have rounded corners, clear but pleasing contrast color to background. Default light mode (white/light gray, buttons in contrast) with dark mode available in menu
	- menu for each page should live in a collapsible bar on the left side, dynamically adaptive to the available options for the user/admin

Integration testing framework
	- project needs automated integration tests that can run in CI to ensure the UI works as expected.
	- write up tests covering the entire existing set of UI flows
	- all future UI flows must have tests as well