// Use-case pages (/use-cases/<slug>): one page per thing a helper does,
// written for people searching for that task. Customer Finder, Morning
// Briefing and Weekend Scout are ready-made helpers on the Shelf; the others
// are built from the same bricks. Same copy rules as copy.ts.
import type { Lang } from '#lib/i18n/index.svelte.ts';
import type { Stack } from './copy.ts';
import { TASK_USE_CASES } from './usecases-tasks.ts';

export interface UseCaseText {
	/** Short name in lists and links. */
	name: string;
	/** The page's headline, phrased like the search. */
	title: string;
	metaTitle: string;
	description: string;
	problem: string;
	how: string;
	stack: Stack;
	/** An example result as it lands in the Feed. */
	result: { title: string; lines: string[] };
	/** What you answer when you set it up. */
	questions: string[];
	notes: string[];
	/** Questions people ask about this task, with short answers. */
	faq?: { q: string; a: string }[];
}

export interface UseCase {
	slug: string;
	avatar: string;
	/** Not built yet: the page says Soon. */
	soon?: boolean;
	/** The public demo (/try) can run it without an account. */
	demo?: boolean;
	text: Record<Lang, UseCaseText>;
}

export const USE_CASES: UseCase[] = [
	{
		slug: 'customer-finder',
		avatar: '🧲',
		text: {
			en: {
				name: 'Customer Finder',
				title: 'Find new customers every week, without the busywork.',
				metaTitle: 'Find local customers every week · Laamo',
				description:
					'A helper that finds businesses who could use what you offer, reads their websites, keeps the good fits and drafts a first email for each. You send the ones you like.',
				problem:
					'Finding new customers means hours of searching maps and websites, copying addresses into a spreadsheet and writing the same email again and again.',
				how: 'Every Monday the Customer Finder looks for businesses of the kind you name, in your city, your region or whole countries. It reads their websites, keeps the ones that fit what you offer and writes a short first email for each. Nothing goes out on its own: you pick, you edit, you send from your own mailbox.',
				stack: [
					{ group: 'when', text: 'Every Monday at 8:00' },
					{ group: 'look', text: 'Find cafés in Leipzig' },
					{ group: 'do', text: 'Save new ones to my Leads list' },
					{ group: 'look', text: 'Read their websites' },
					{ group: 'think', text: 'Pull out email and phone' },
					{ group: 'think', text: 'Keep the ones without online booking' },
					{ group: 'ask', text: 'Let me pick' },
					{ group: 'think', text: 'Write a short first email' },
					{ group: 'ask', text: 'Check first' }
				],
				result: {
					title: 'Found 8 cafés in Leipzig',
					lines: [
						'Café Rosental: no online booking, email on the contact page',
						'Kaffeehaus Riquet: small website, phone only',
						'6 more to swipe through, with a draft for each'
					]
				},
				questions: [
					'What kind of businesses should I look for?',
					'Where should I look? A city, a region, countries or the EU.',
					'What do you offer them? (asked once, remembered for all helpers)'
				],
				notes: [
					'Every business lands in your Leads list, so nobody gets a second first email.',
					'First emails are drafts. Rules for cold emails differ by country; sending them from your own mailbox keeps you in charge.'
				]
			},
			de: {
				name: 'Kundenfinder',
				title: 'Jede Woche neue Kunden finden, ohne die Fleißarbeit.',
				metaTitle: 'Jede Woche lokale Kunden finden · Laamo',
				description:
					'Ein Helfer, der Betriebe findet, die dein Angebot brauchen könnten, ihre Websites liest, die passenden behält und für jeden eine erste E-Mail entwirft. Du schickst die, die dir gefallen.',
				problem:
					'Neue Kunden finden heißt: stundenlang Karten und Websites durchsuchen, Adressen in eine Tabelle kopieren und immer wieder dieselbe E-Mail schreiben.',
				how: 'Jeden Montag sucht der Kundenfinder Betriebe der Art, die du nennst, in deiner Stadt, deiner Region oder in ganzen Ländern. Er liest ihre Websites, behält die, die zu deinem Angebot passen, und schreibt für jeden eine kurze erste E-Mail. Nichts geht von allein raus: Du wählst aus, du änderst, du schickst aus deinem eigenen Postfach.',
				stack: [
					{ group: 'when', text: 'Jeden Montag um 8:00' },
					{ group: 'look', text: 'Cafés in Leipzig finden' },
					{ group: 'do', text: 'Neue in meiner Leads-Liste speichern' },
					{ group: 'look', text: 'Ihre Websites lesen' },
					{ group: 'think', text: 'E-Mail und Telefon herausziehen' },
					{ group: 'think', text: 'Die ohne Online-Buchung behalten' },
					{ group: 'ask', text: 'Lass mich wählen' },
					{ group: 'think', text: 'Eine kurze erste E-Mail schreiben' },
					{ group: 'ask', text: 'Erst prüfen' }
				],
				result: {
					title: '8 Cafés in Leipzig gefunden',
					lines: [
						'Café Rosental: keine Online-Buchung, E-Mail auf der Kontaktseite',
						'Kaffeehaus Riquet: kleine Website, nur Telefon',
						'6 weitere zum Durchwischen, mit einem Entwurf für jedes'
					]
				},
				questions: [
					'Nach welchen Betrieben soll ich suchen?',
					'Wo soll ich suchen? Eine Stadt, eine Region, Länder oder die EU.',
					'Was bietest du ihnen an? (einmal gefragt, für alle Helfer gemerkt)'
				],
				notes: [
					'Jeder Betrieb landet in deiner Leads-Liste, so bekommt niemand eine zweite erste E-Mail.',
					'Erste E-Mails sind Entwürfe. Die Regeln für Kaltakquise unterscheiden sich je nach Land; wenn du aus deinem eigenen Postfach schickst, behältst du die Kontrolle.'
				]
			}
		}
	},
	{
		slug: 'competitor-research',
		avatar: '👀',
		text: {
			en: {
				name: 'Competitor research',
				title: 'Keep an eye on your competitors, in five lines a week.',
				metaTitle: 'Weekly competitor research summary · Laamo',
				description:
					'A helper that searches what your competitors are up to every week, keeps price changes and new offers, and sends you a short summary in your chat.',
				problem:
					'You know you should watch your competitors, but checking their websites and the news every week never makes it onto the list.',
				how: 'Every Friday the helper searches the web for news about the competitors you name. It keeps what matters to you, like new prices, new offers or a new shop, sums it up in a few lines and posts it to your chat. Change what counts as important in plain words.',
				stack: [
					{ group: 'when', text: 'Every Friday at 9:00' },
					{ group: 'look', text: 'Search news about my competitors' },
					{ group: 'think', text: 'Keep price changes and new offers' },
					{ group: 'think', text: 'Sum it up in 5 lines' },
					{ group: 'do', text: 'Post it to my chat' }
				],
				result: {
					title: 'Your competitors this week',
					lines: [
						'Bike Hub Leipzig lowered service prices by 10% until March.',
						'Radwerk opened a second shop in Plagwitz.',
						'Nothing new from the other three.'
					]
				},
				questions: [
					'Which competitors should I watch?',
					'What counts as important? (e.g. prices, new offers)',
					'Where should I tell you?'
				],
				notes: [
					'Built from bricks you can change: search more often, keep more, or save every find to a list.'
				]
			},
			de: {
				name: 'Mitbewerber-Recherche',
				title: 'Behalte deine Mitbewerber im Blick, in fünf Zeilen pro Woche.',
				metaTitle: 'Wöchentliche Mitbewerber-Recherche · Laamo',
				description:
					'Ein Helfer, der jede Woche sucht, was deine Mitbewerber vorhaben, Preisänderungen und neue Angebote behält und dir eine kurze Zusammenfassung in deinen Chat schickt.',
				problem:
					'Du weißt, dass du deine Mitbewerber beobachten solltest, aber jede Woche ihre Websites und die Nachrichten durchzusehen, schafft es nie auf die Liste.',
				how: 'Jeden Freitag sucht der Helfer im Web nach Neuigkeiten zu den Mitbewerbern, die du nennst. Er behält, was für dich zählt, etwa neue Preise, neue Angebote oder ein neues Geschäft, fasst es in wenigen Zeilen zusammen und postet es in deinen Chat. Was wichtig ist, legst du in einfachen Worten fest.',
				stack: [
					{ group: 'when', text: 'Jeden Freitag um 9:00' },
					{ group: 'look', text: 'Neuigkeiten zu meinen Mitbewerbern suchen' },
					{ group: 'think', text: 'Preisänderungen und neue Angebote behalten' },
					{ group: 'think', text: 'In 5 Zeilen zusammenfassen' },
					{ group: 'do', text: 'In meinen Chat posten' }
				],
				result: {
					title: 'Deine Mitbewerber diese Woche',
					lines: [
						'Bike Hub Leipzig senkt die Service-Preise bis März um 10 %.',
						'Radwerk hat ein zweites Geschäft in Plagwitz eröffnet.',
						'Von den anderen drei nichts Neues.'
					]
				},
				questions: [
					'Welche Mitbewerber soll ich beobachten?',
					'Was ist wichtig? (z. B. Preise, neue Angebote)',
					'Wo soll ich dir Bescheid geben?'
				],
				notes: [
					'Aus Bausteinen gebaut, die du ändern kannst: öfter suchen, mehr behalten oder jeden Fund in einer Liste speichern.'
				]
			}
		}
	},
	{
		slug: 'morning-briefing',
		avatar: '📰',
		demo: true,
		text: {
			en: {
				name: 'Morning Briefing',
				title: 'The news that matters for your business, every morning.',
				metaTitle: 'Daily local news summary for your business · Laamo',
				description:
					'A helper that reads the local news every weekday, keeps the 5 stories that matter for your business and sends you a short summary in your chat at 7:30.',
				problem:
					'There is too much news to read and most of it has nothing to do with your business. The few stories that do are easy to miss.',
				how: 'Every weekday at 7:30 the Morning Briefing searches the news from the last day for your kind of business in your city. It keeps the 5 stories that matter most, sums them up in a few lines and posts them to your chat, with links if you want to read more.',
				stack: [
					{ group: 'when', text: 'Every weekday at 7:30' },
					{ group: 'look', text: 'Search the news for bakeries in Leipzig' },
					{ group: 'think', text: 'Keep the 5 that matter' },
					{ group: 'think', text: 'Sum them up in 5 lines' },
					{ group: 'do', text: 'Post it to my team chat' }
				],
				result: {
					title: 'Your morning briefing',
					lines: [
						'The city plans a new market hall in the west of Leipzig.',
						'Flour prices are expected to stay flat this spring.',
						'3 more stories, with links'
					]
				},
				questions: [
					'What kind of business do you have? (remembered)',
					'Which city? (remembered)',
					'Where should I tell you?'
				],
				notes: ['Try it now without an account: pick Morning Briefing on the demo page.']
			},
			de: {
				name: 'Morgen-Briefing',
				title: 'Die Nachrichten, die für dein Geschäft zählen, jeden Morgen.',
				metaTitle: 'Tägliche lokale Nachrichten für dein Geschäft · Laamo',
				description:
					'Ein Helfer, der jeden Werktag die lokalen Nachrichten liest, die 5 wichtigsten für dein Geschäft behält und dir um 7:30 eine kurze Zusammenfassung in deinen Chat schickt.',
				problem:
					'Es gibt zu viele Nachrichten, und die meisten haben nichts mit deinem Geschäft zu tun. Die wenigen, die es betreffen, gehen leicht unter.',
				how: 'Jeden Werktag um 7:30 sucht das Morgen-Briefing die Nachrichten des letzten Tages zu deiner Branche in deiner Stadt. Es behält die 5 wichtigsten, fasst sie in wenigen Zeilen zusammen und postet sie in deinen Chat, mit Links, wenn du mehr lesen willst.',
				stack: [
					{ group: 'when', text: 'Jeden Werktag um 7:30' },
					{ group: 'look', text: 'Nachrichten zu Bäckereien in Leipzig suchen' },
					{ group: 'think', text: 'Die 5 wichtigsten behalten' },
					{ group: 'think', text: 'In 5 Zeilen zusammenfassen' },
					{ group: 'do', text: 'In meinen Team-Chat posten' }
				],
				result: {
					title: 'Dein Morgen-Briefing',
					lines: [
						'Die Stadt plant eine neue Markthalle im Leipziger Westen.',
						'Die Mehlpreise sollen im Frühjahr stabil bleiben.',
						'3 weitere Meldungen, mit Links'
					]
				},
				questions: [
					'Was für ein Geschäft hast du? (wird gemerkt)',
					'Welche Stadt? (wird gemerkt)',
					'Wo soll ich dir Bescheid geben?'
				],
				notes: ['Probier es jetzt ohne Konto aus: Wähle das Morgen-Briefing auf der Demo-Seite.']
			}
		}
	},
	{
		slug: 'inbox-helper',
		avatar: '📥',
		text: {
			en: {
				name: 'Inbox Helper',
				title: 'Forward your emails, get the urgent ones in one line each.',
				metaTitle: 'Summarize your emails into one line each · Laamo',
				description:
					'Give a helper its own email address and forward mail to it. It keeps the urgent ones and sends you a one-line summary of each in your chat.',
				problem:
					'Your inbox mixes the three emails that need you today with fifty that can wait. Finding the three takes the first hour of the day.',
				how: 'The Inbox Helper gets its own email address. Forward mail to it, or set a forwarding rule in your mail app. For every email it decides whether it is urgent, sums it up in one line and tells you in your chat. You can limit it to emails from certain senders.',
				stack: [
					{ group: 'when', text: 'When an email arrives' },
					{ group: 'think', text: 'Keep the urgent ones' },
					{ group: 'think', text: 'Sum it up in one line' },
					{ group: 'do', text: 'Post it to my chat' }
				],
				result: {
					title: 'Urgent: delivery moved to Thursday',
					lines: [
						'Your flour supplier can only deliver on Thursday this week and asks you to confirm by noon.'
					]
				},
				questions: ['Which emails count as urgent?', 'Where should I tell you?'],
				notes: [
					'The helper reads only the emails you forward to its own address. It never gets access to your mailbox.'
				]
			},
			de: {
				name: 'Postfach-Helfer',
				title: 'Leite deine E-Mails weiter, bekomm die dringenden in je einer Zeile.',
				metaTitle: 'E-Mails in je einer Zeile zusammenfassen · Laamo',
				description:
					'Gib einem Helfer eine eigene E-Mail-Adresse und leite Mails dorthin weiter. Er behält die dringenden und schickt dir für jede eine Zusammenfassung in einer Zeile in deinen Chat.',
				problem:
					'In deinem Postfach stecken die drei E-Mails, die heute dich brauchen, zwischen fünfzig, die warten können. Die drei zu finden kostet die erste Stunde des Tages.',
				how: 'Der Postfach-Helfer bekommt eine eigene E-Mail-Adresse. Leite Mails dorthin weiter oder richte in deinem Mailprogramm eine Weiterleitung ein. Für jede E-Mail entscheidet er, ob sie dringend ist, fasst sie in einer Zeile zusammen und sagt dir im Chat Bescheid. Du kannst ihn auf bestimmte Absender beschränken.',
				stack: [
					{ group: 'when', text: 'Wenn eine E-Mail ankommt' },
					{ group: 'think', text: 'Die dringenden behalten' },
					{ group: 'think', text: 'In einer Zeile zusammenfassen' },
					{ group: 'do', text: 'In meinen Chat posten' }
				],
				result: {
					title: 'Dringend: Lieferung auf Donnerstag verschoben',
					lines: [
						'Dein Mehllieferant kann diese Woche nur am Donnerstag liefern und bittet bis 12 Uhr um Bestätigung.'
					]
				},
				questions: ['Welche E-Mails sind dringend?', 'Wo soll ich dir Bescheid geben?'],
				notes: [
					'Der Helfer liest nur die E-Mails, die du an seine eigene Adresse weiterleitest. Auf dein Postfach hat er keinen Zugriff.'
				]
			}
		}
	},
	{
		slug: 'social-poster',
		avatar: '📣',
		soon: true,
		text: {
			en: {
				name: 'Social Poster',
				title: 'Social posts for your business, written and scheduled for you.',
				metaTitle: 'Write and schedule social media posts for your business · Laamo',
				description:
					'A helper that finds something worth sharing, writes a post in your tone, draws a picture for it and, after your OK, publishes it to your social accounts with Posta.',
				problem:
					'Posting regularly keeps your business visible, but finding a topic, writing the text and making a picture eats an evening every week.',
				how: 'Twice a week the Social Poster looks for news in your field, writes a post in your tone with fitting hashtags and draws a picture for it. You check the post in your Feed, change what you like, and Posta publishes it to your social accounts at the time you pick.',
				stack: [
					{ group: 'when', text: 'Every Tuesday and Friday at 10:00' },
					{ group: 'look', text: 'Search news in my field' },
					{ group: 'think', text: 'Keep the best one' },
					{ group: 'think', text: 'Write a post in my tone' },
					{ group: 'think', text: 'Draw a picture for it' },
					{ group: 'ask', text: 'Check first' },
					{ group: 'do', text: 'Post with Posta at 12:00' }
				],
				result: {
					title: 'A post is ready for you to check',
					lines: [
						'"Spring is here, and so is our rhubarb cake. Fresh from Thursday, while it lasts. #Leipzig #bakery"',
						'With a picture. Approve, edit or skip.'
					]
				},
				questions: [
					'What is your business about? (remembered)',
					'Which social accounts?',
					'When should posts go out?'
				],
				notes: ['Writing posts and pictures works today; publishing with Posta is coming soon.']
			},
			de: {
				name: 'Social-Poster',
				title: 'Social-Media-Beiträge für dein Geschäft, geschrieben und geplant.',
				metaTitle: 'Social-Media-Beiträge für dein Geschäft schreiben und planen · Laamo',
				description:
					'Ein Helfer, der etwas Teilenswertes findet, einen Beitrag in deinem Ton schreibt, ein Bild dazu zeichnet und ihn nach deinem OK mit Posta auf deinen Social-Media-Konten veröffentlicht.',
				problem:
					'Regelmäßig posten hält dein Geschäft sichtbar, aber Thema finden, Text schreiben und Bild machen kostet jede Woche einen Abend.',
				how: 'Zweimal pro Woche sucht der Social-Poster Neuigkeiten aus deiner Branche, schreibt einen Beitrag in deinem Ton mit passenden Hashtags und zeichnet ein Bild dazu. Du prüfst den Beitrag in deinem Feed, änderst, was du willst, und Posta veröffentlicht ihn zur gewählten Zeit auf deinen Konten.',
				stack: [
					{ group: 'when', text: 'Jeden Dienstag und Freitag um 10:00' },
					{ group: 'look', text: 'Neuigkeiten aus meiner Branche suchen' },
					{ group: 'think', text: 'Die beste behalten' },
					{ group: 'think', text: 'Einen Beitrag in meinem Ton schreiben' },
					{ group: 'think', text: 'Ein Bild dazu zeichnen' },
					{ group: 'ask', text: 'Erst prüfen' },
					{ group: 'do', text: 'Mit Posta um 12:00 posten' }
				],
				result: {
					title: 'Ein Beitrag wartet auf dich',
					lines: [
						'„Der Frühling ist da, und unser Rhabarberkuchen auch. Frisch ab Donnerstag, solange er reicht. #Leipzig #Bäckerei"',
						'Mit Bild. Freigeben, ändern oder überspringen.'
					]
				},
				questions: [
					'Worum geht es in deinem Geschäft? (wird gemerkt)',
					'Welche Social-Media-Konten?',
					'Wann sollen Beiträge erscheinen?'
				],
				notes: [
					'Beiträge und Bilder schreiben geht heute schon; das Veröffentlichen mit Posta kommt bald.'
				]
			}
		}
	},
	{
		slug: 'weekend-scout',
		avatar: '🎟️',
		demo: true,
		text: {
			en: {
				name: 'Weekend Scout',
				title: 'Things to do in your city this weekend, picked for you.',
				metaTitle: 'Things to do in your city this weekend · Laamo',
				description:
					'A helper that searches events in your city every Thursday evening. Swipe to keep the ones you like.',
				problem:
					'The good concerts, markets and openings are spread over a dozen websites, and you hear about them on Monday.',
				how: 'Every Thursday at 18:00 the Weekend Scout searches what is on in your city this weekend, keeps the events that fit what you like and shows them as cards. Swipe right to keep, left to skip. It learns from your swipes.',
				stack: [
					{ group: 'when', text: 'Every Thursday at 18:00' },
					{ group: 'look', text: 'Search events in Leipzig this weekend' },
					{ group: 'think', text: 'Keep concerts, markets and food' },
					{ group: 'ask', text: 'Let me pick' }
				],
				result: {
					title: '9 things to do this weekend',
					lines: [
						'Saturday: spring market at Lindenauer Markt',
						'Sunday: jazz brunch in Plagwitz',
						'7 more to swipe through'
					]
				},
				questions: ['Which city? (remembered)'],
				notes: ['Try it now without an account: pick Weekend Scout on the demo page.']
			},
			de: {
				name: 'Wochenend-Scout',
				title: 'Was am Wochenende in deiner Stadt los ist, für dich ausgesucht.',
				metaTitle: 'Was am Wochenende in deiner Stadt los ist · Laamo',
				description:
					'Ein Helfer, der jeden Donnerstagabend Veranstaltungen in deiner Stadt sucht. Wisch, um die zu behalten, die dir gefallen.',
				problem:
					'Die guten Konzerte, Märkte und Eröffnungen sind über ein Dutzend Websites verteilt, und du hörst am Montag davon.',
				how: 'Jeden Donnerstag um 18:00 sucht der Wochenend-Scout, was am Wochenende in deiner Stadt los ist, behält die Veranstaltungen, die zu dir passen, und zeigt sie als Karten. Nach rechts wischen behält, nach links überspringt. Er lernt aus deinen Wischern.',
				stack: [
					{ group: 'when', text: 'Jeden Donnerstag um 18:00' },
					{ group: 'look', text: 'Veranstaltungen in Leipzig am Wochenende suchen' },
					{ group: 'think', text: 'Konzerte, Märkte und Essen behalten' },
					{ group: 'ask', text: 'Lass mich wählen' }
				],
				result: {
					title: '9 Ideen fürs Wochenende',
					lines: [
						'Samstag: Frühlingsmarkt am Lindenauer Markt',
						'Sonntag: Jazz-Brunch in Plagwitz',
						'7 weitere zum Durchwischen'
					]
				},
				questions: ['Welche Stadt? (wird gemerkt)'],
				notes: ['Probier es jetzt ohne Konto aus: Wähle den Wochenend-Scout auf der Demo-Seite.']
			}
		}
	},
	...TASK_USE_CASES
];

export function useCase(slug: string): UseCase | undefined {
	return USE_CASES.find((u) => u.slug === slug);
}

/** Labels for the use-case pages, per language. */
export const UI: Record<
	Lang,
	{
		listTitle: string;
		listMeta: string;
		listLead: string;
		/** What a helper is, in a few paragraphs (the list page). */
		whatTitle: string;
		what: string[];
		problem: string;
		how: string;
		result: string;
		setup: string;
		faq: string;
		others: string;
		join: string;
		joinBody: string;
		tryIt: string;
		all: string;
	}
> = {
	en: {
		listTitle: 'What a helper can do for you',
		listMeta:
			'Helpers for small businesses: find customers, watch competitors, sum up news and emails · Laamo',
		listLead:
			'Each helper is a short stack of bricks you can change. Start with one of these, or describe your own.',
		whatTitle: 'What is a helper?',
		what: [
			'A helper is a small AI agent for one recurring task. It is a short stack of bricks: when it runs, where it looks, what it thinks about, what it does with the result, and when it asks you first.',
			'You don’t write prompts or connect tools. You pick a helper, answer up to three questions in your own words, and it runs on its schedule. The result lands in your Feed, in your chat or in a list you can download.',
			'Every brick can be changed, and you can stack your own from nothing. Anything a helper wants to send to someone else waits for your OK.'
		],
		problem: 'The problem',
		how: 'How the helper does it',
		result: 'What lands in your Feed',
		setup: 'You set it up with',
		faq: 'Questions',
		others: 'More helpers',
		join: 'Get your invite',
		joinBody: "Join the waiting list and we'll write when it's your turn.",
		tryIt: 'Try it now, no account',
		all: 'All helpers'
	},
	de: {
		listTitle: 'Was ein Helfer für dich tun kann',
		listMeta:
			'Helfer für kleine Unternehmen: Kunden finden, Mitbewerber beobachten, Nachrichten und E-Mails zusammenfassen · Laamo',
		listLead:
			'Jeder Helfer ist ein kurzer Stapel Bausteine, den du ändern kannst. Fang mit einem davon an oder beschreib deinen eigenen.',
		whatTitle: 'Was ist ein Helfer?',
		what: [
			'Ein Helfer ist ein kleiner KI-Agent für eine wiederkehrende Aufgabe. Er ist ein kurzer Stapel Bausteine: wann er läuft, wo er nachsieht, worüber er nachdenkt, was er mit dem Ergebnis tut und wann er dich erst fragt.',
			'Du schreibst keine Prompts und verbindest keine Werkzeuge. Du suchst dir einen Helfer aus, beantwortest bis zu drei Fragen in deinen eigenen Worten, und er läuft nach seinem Zeitplan. Das Ergebnis landet in deinem Feed, in deinem Chat oder in einer Liste zum Herunterladen.',
			'Jeder Baustein lässt sich ändern, und du kannst deinen eigenen Stapel von null aufbauen. Alles, was ein Helfer an jemand anderen schicken will, wartet auf dein OK.'
		],
		problem: 'Das Problem',
		how: 'So macht es der Helfer',
		result: 'Was in deinem Feed landet',
		setup: 'Du richtest ihn ein mit',
		faq: 'Fragen',
		others: 'Weitere Helfer',
		join: 'Hol dir deine Einladung',
		joinBody: 'Trag dich in die Warteliste ein, und wir schreiben dir, wenn du dran bist.',
		tryIt: 'Jetzt ausprobieren, ohne Konto',
		all: 'Alle Helfer'
	}
};
