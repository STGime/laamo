import type { Key } from './en';

// Deutsch. Du-Form, kurz und konkret.
export const de: Record<Key, string> = {
	'tab.feed': 'Feed',
	'tab.helpers': 'Helfer',
	'tab.new': 'Neuer Helfer',
	'tab.lists': 'Listen',
	'tab.me': 'Ich',

	'feed.title': 'Feed',
	'feed.filter.all': 'Alle',
	'feed.filter.needsMe': 'Braucht mich',
	'feed.filter.results': 'Ergebnisse',
	'feed.empty.title': 'Noch nichts da',
	'feed.empty.body': 'Wenn ein Helfer etwas findet, landet es hier. Zum Beispiel so:',
	'feed.empty.exampleHelper': 'Morgen-Briefing',
	'feed.empty.exampleTitle': '5 Dinge für deine Bäckerei heute',
	'feed.empty.exampleBody':
		'Mehl wird im Dezember 8 % teurer. Der Weihnachtsmarkt öffnet am Freitag.',
	'feed.empty.cta': 'Meinen ersten Helfer bauen',

	'helpers.title': 'Helfer',
	'helpers.empty': 'Noch keine Helfer. Nimm einen aus dem Regal oder bau deinen eigenen.',
	'new.title': 'Neuer Helfer',
	'new.shelf': 'Aus dem Regal nehmen',
	'new.build': 'Stein für Stein bauen',
	'new.soon': 'Kommt bald',
	'lists.title': 'Listen',
	'lists.empty': 'Hier können Helfer Firmen, Kontakte und Links speichern.',
	'me.title': 'Ich',
	'me.signedInAs': 'Angemeldet als {email}',
	'me.workspace': 'Workspace',
	'me.language': 'Sprache',
	'me.signOut': 'Abmelden',

	'bricks.when': 'Wann',
	'bricks.look': 'Schauen',
	'bricks.think': 'Denken',
	'bricks.do': 'Tun',
	'bricks.ask': 'Frag mich',

	'sheet.close': 'Schließen',
	'swipe.keep': 'Behalten',
	'swipe.skip': 'Überspringen',
	'swipe.left': 'noch {n}',
	'swipe.done': 'Alles erledigt',

	'signin.title': 'Anmelden',
	'signin.devHint': 'Nur für Entwicklung: Zugangstoken aus scripts/dev-token.sh einfügen.',
	'signin.token': 'Zugangstoken',
	'signin.go': 'Anmelden',
	'signin.bad': 'Das Token hat nicht funktioniert. Tokens gelten eine Stunde.',

	'error.offline': 'Laamo ist gerade nicht erreichbar. Versuch es gleich noch mal.',
	loading: 'Lädt…'
};
