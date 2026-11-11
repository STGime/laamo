// English texts. Keys are shared with de.ts (a test checks they match).
// Voice: short, friendly, concrete. No buzzwords.
export const en = {
	'tab.feed': 'Feed',
	'tab.helpers': 'Helpers',
	'tab.new': 'New helper',
	'tab.lists': 'Lists',
	'tab.me': 'Me',

	'feed.title': 'Feed',
	'feed.filter.all': 'All',
	'feed.filter.needsMe': 'Needs me',
	'feed.filter.results': 'Results',
	'feed.empty.title': 'Nothing here yet',
	'feed.empty.body': 'When a helper finds something, it lands here. Like this:',
	'feed.empty.exampleHelper': 'Morning Briefing',
	'feed.empty.exampleTitle': '5 things for your bakery today',
	'feed.empty.exampleBody':
		'Flour prices rise 8% in December. The Christmas market opens on Friday.',
	'feed.empty.cta': 'Make my first helper',

	'helpers.title': 'Helpers',
	'helpers.empty': 'No helpers yet. Pick one from the shelf or build your own.',
	'new.title': 'New helper',
	'new.shelf': 'Pick from the shelf',
	'new.build': 'Build it brick by brick',
	'new.soon': 'Coming soon',
	'lists.title': 'Lists',
	'lists.empty': 'Helpers can save businesses, leads and links here.',
	'me.title': 'Me',
	'me.signedInAs': 'Signed in as {email}',
	'me.workspace': 'Workspace',
	'me.language': 'Language',
	'me.signOut': 'Sign out',

	'bricks.when': 'When',
	'bricks.look': 'Look',
	'bricks.think': 'Think',
	'bricks.do': 'Do',
	'bricks.ask': 'Ask me',

	'sheet.close': 'Close',
	'swipe.keep': 'Keep',
	'swipe.skip': 'Skip',
	'swipe.left': '{n} left',
	'swipe.done': 'All done',

	'signin.title': 'Sign in',
	'signin.devHint': 'Dev only: paste an access token from scripts/dev-token.sh.',
	'signin.token': 'Access token',
	'signin.go': 'Sign in',
	'signin.bad': 'That token didn’t work. Tokens last one hour.',

	'error.offline': 'Can’t reach Laamo right now. Try again in a moment.',
	loading: 'Loading…'
} as const;

export type Key = keyof typeof en;
