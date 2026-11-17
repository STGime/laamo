// The public site's words, English and German side by side. Rules (marketing
// plan): no names of other tools, platforms or AI companies (Posta is ours);
// no AI buzzwords; short sentences; anything not built yet says "soon".
import type { Group } from '#lib/components/Brick.svelte';
import type { Lang } from '#lib/i18n/index.svelte.ts';

export type Stack = { group: Group; text: string }[];

export interface Copy {
	meta: { title: string; description: string };
	hero: { title: string; sub: string };
	form: {
		label: string;
		placeholder: string;
		interests: string;
		interest: Record<string, string>;
		wish: string;
		wishPlaceholder: string;
		button: string;
		working: string;
		consent: string;
		sent: (email: string) => string;
		sentHint: string;
		again: string;
		badEmail: string;
		slow: string;
		failed: string;
		noScript: string;
	};
	problem: { title: string; body: string[]; stackTitle: string; stack: Stack };
	how: {
		title: string;
		steps: { title: string; body: string }[];
		bricksTitle: string;
		bricks: { group: Group; name: string; question: string }[];
	};
	mobile: { title: string; body: string };
	social: { title: string; soon: string; body: string; stack: Stack };
	customers: {
		title: string;
		examples: { name: string; body: string; stack: Stack }[];
		note: string;
	};
	fast: { title: string; steps: { time: string; text: string }[]; note: string };
	free: { title: string; points: { text: string; soon?: boolean }[] };
	soon: string;
	tryIt: { title: string; body: string; button: string };
	faq: { title: string; items: { q: string; a: string }[] };
	end: { title: string; body: string };
	confirm: {
		metaTitle: string;
		title: string;
		body: string;
		button: string;
		working: string;
		doneTitle: string;
		doneBody: string;
		shareTitle: string;
		shareBody: string;
		copy: string;
		copied: string;
		share: string;
		badTitle: string;
		badBody: string;
		home: string;
		noLink: string;
	};
}

const en: Copy = {
	meta: {
		title: 'Laamo: agents that do your busywork, set up in a few taps',
		description:
			'Pick a helper, answer three questions, and it does a daily task for you: finds customers, watches competitors, sums up the news, writes posts. On your phone or computer.'
	},
	hero: {
		title: 'Agents that do your busywork. Set up in a few taps.',
		sub: "You've heard about AI agents. Building one shouldn't feel like programming. Pick a helper, answer three questions, and it gets to work, on your phone or your computer."
	},
	form: {
		label: 'Your email',
		placeholder: 'you@example.com',
		interests: 'What should a helper do for you? (optional)',
		interest: {
			customers: 'Find customers',
			competitors: 'Watch competitors',
			social: 'Write social posts',
			news: 'Sum up the news',
			emails: 'Sort my emails',
			events: 'Plan events',
			other: 'Something else'
		},
		wish: 'What should it do?',
		wishPlaceholder: 'e.g. check my supplier prices every week',
		button: 'Join the waiting list',
		working: 'One moment…',
		consent:
			"We'll email you a link to confirm. After that we write when your invite is ready, and now and then about what's new in Laamo. You can leave the list anytime.",
		sent: (email) => `Check your inbox: we sent a link to ${email}.`,
		sentHint: 'Tap it to confirm your spot. Nothing there? Look in your spam folder.',
		again: 'Use another address',
		badEmail: "That doesn't look like an email address.",
		slow: 'Lots of sign-ups right now. Please try again in a few minutes.',
		failed: "That didn't work. Please try again in a minute.",
		noScript: 'The sign-up form needs JavaScript.'
	},
	problem: {
		title: "Agents shouldn't need an engineer.",
		body: [
			'Most automation tools look like a wiring diagram: boxes, arrows, keys, error logs. Many people give up before the first result.',
			'Laamo looks like the apps you use every day. A helper is a short stack of colored bricks, and each brick is one plain sentence.'
		],
		stackTitle: 'A whole helper:',
		stack: [
			{ group: 'when', text: 'Every weekday at 7:30' },
			{ group: 'look', text: 'Search the news for bakeries in Leipzig' },
			{ group: 'think', text: 'Keep the 5 that matter' },
			{ group: 'think', text: 'Sum them up in 5 lines' },
			{ group: 'do', text: 'Post it to my team chat' }
		]
	},
	how: {
		title: 'How it works',
		steps: [
			{
				title: 'Pick a helper',
				body: 'Start from a ready-made helper, or describe in your own words what you need.'
			},
			{
				title: 'Answer three questions',
				body: 'Like your city, your business and where to tell you. Laamo remembers the answers for your next helpers.'
			},
			{
				title: 'Results land in your Feed',
				body: 'Swipe through what it found. Anything that leaves Laamo waits for your OK first.'
			}
		],
		bricksTitle: 'Five colors, five questions',
		bricks: [
			{ group: 'when', name: 'When', question: 'When should it start?' },
			{ group: 'look', name: 'Look', question: 'What should it read?' },
			{ group: 'think', name: 'Think', question: 'What should it do with it?' },
			{ group: 'do', name: 'Do', question: 'What should change?' },
			{ group: 'ask', name: 'Ask me', question: 'When does it need your OK?' }
		]
	},
	mobile: {
		title: 'Build it on the bus.',
		body: 'Laamo works the same on your phone and your computer. Set up a helper in your browser, approve results with a swipe, and add Laamo to your home screen. Apps for your phone are coming soon.'
	},
	social: {
		title: 'Post to your social accounts with Posta.',
		soon: 'Soon',
		body: 'A helper writes the post and draws a picture for it. You check it, and Posta publishes it to your social accounts, right away or at a time you pick.',
		stack: [
			{ group: 'when', text: 'Every Tuesday and Friday at 10:00' },
			{ group: 'look', text: 'Search news in my field' },
			{ group: 'think', text: 'Write a post in my tone' },
			{ group: 'think', text: 'Draw a picture for it' },
			{ group: 'ask', text: 'Check first' },
			{ group: 'do', text: 'Post with Posta at 12:00' }
		]
	},
	customers: {
		title: 'Find customers. Keep an eye on competitors.',
		examples: [
			{
				name: 'Customer Finder',
				body: 'Every Monday it finds cafés in your city, reads their websites, keeps the ones without online booking and writes a first email for each.',
				stack: [
					{ group: 'when', text: 'Every Monday at 8:00' },
					{ group: 'look', text: 'Find cafés in Leipzig' },
					{ group: 'look', text: 'Read their websites' },
					{ group: 'think', text: 'Keep the ones without online booking' },
					{ group: 'ask', text: 'Let me pick' },
					{ group: 'think', text: 'Write a short first email' }
				]
			},
			{
				name: 'Competitor research',
				body: 'Every week it searches what your competitors are up to, keeps what matters and sends you a short summary.',
				stack: [
					{ group: 'when', text: 'Every Friday at 9:00' },
					{ group: 'look', text: 'Search news about my competitors' },
					{ group: 'think', text: 'Keep price changes and new offers' },
					{ group: 'think', text: 'Sum it up' },
					{ group: 'do', text: 'Post it to my chat' }
				]
			}
		],
		note: 'First emails stay drafts. You send the ones you like from your own mailbox.'
	},
	fast: {
		title: 'Your first result in 5 minutes.',
		steps: [
			{ time: '0:30', text: 'Sign up with your email' },
			{ time: '1:30', text: 'Pick a helper' },
			{ time: '2:30', text: 'Answer three questions' },
			{ time: '5:00', text: 'Its first result is in your Feed' }
		],
		note: 'No setup calls, no keys to copy, no manual to read.'
	},
	free: {
		title: 'No lock-in.',
		points: [
			{ text: 'Download every list as a spreadsheet file (CSV), anytime.' },
			{ text: 'Your data is stored in the EU.' },
			{ text: 'An AI model is included. Bring your own model if you prefer.', soon: true }
		]
	},
	soon: 'Soon',
	tryIt: {
		title: 'Try a helper now.',
		body: 'No account, no email. Pick a sample helper and see its result in under a minute.',
		button: 'Try it now'
	},
	faq: {
		title: 'Questions',
		items: [
			{
				q: 'Do I need to know how to code?',
				a: 'No. You pick bricks and answer questions in plain words. If you can use a phone, you can build a helper.'
			},
			{
				q: 'What does it cost?',
				a: "There will be a free plan to start with. We'll share the prices before your invite arrives."
			},
			{
				q: 'Will a helper send things without asking me?',
				a: 'Not unless you say so. Anything that leaves Laamo waits for your OK by default, and emails to new contacts are always drafts you send yourself.'
			},
			{
				q: 'Where is my data?',
				a: 'Stored in the EU. Services a helper uses for a task, like a web search or an AI model, only get what they need for that task.'
			},
			{
				q: 'Which AI does Laamo use?',
				a: 'Every plan includes an AI model, so you need no account anywhere else. Soon you can bring your own model.'
			},
			{
				q: 'Is there an app?',
				a: 'Laamo runs in the browser on your phone and computer, and you can add it to your home screen. Apps for your phone are coming soon.'
			}
		]
	},
	end: {
		title: 'Get your invite.',
		body: "We're letting people in a few at a time. Join the list and we'll write when it's your turn."
	},
	confirm: {
		metaTitle: 'Confirm your spot · Laamo',
		title: 'One tap to confirm.',
		body: 'Confirm that you want to join the Laamo waiting list.',
		button: 'Yes, put me on the list',
		working: 'One moment…',
		doneTitle: "You're on the list.",
		doneBody: "We'll email you when your invite is ready.",
		shareTitle: 'Move up the list',
		shareBody: 'Share your link. Everyone who joins with it moves you up.',
		copy: 'Copy link',
		copied: 'Copied',
		share: 'Share',
		badTitle: "This link doesn't work anymore.",
		badBody: 'It may be older than 7 days or already used. Sign up again and we send a new one.',
		home: 'Back to Laamo',
		noLink: 'This page needs the link from your email.'
	}
};

const de: Copy = {
	meta: {
		title: 'Laamo: Agenten, die dir Kleinkram abnehmen, mit ein paar Taps eingerichtet',
		description:
			'Such dir einen Helfer aus, beantworte drei Fragen, und er erledigt eine tägliche Aufgabe für dich: findet Kunden, beobachtet Mitbewerber, fasst Neuigkeiten zusammen, schreibt Beiträge. Am Handy oder Computer.'
	},
	hero: {
		title: 'Agenten, die dir Kleinkram abnehmen. Mit ein paar Taps eingerichtet.',
		sub: 'Alle reden von KI-Agenten. Einen zu bauen sollte sich nicht wie Programmieren anfühlen. Such dir einen Helfer aus, beantworte drei Fragen, und er legt los, am Handy oder am Computer.'
	},
	form: {
		label: 'Deine E-Mail',
		placeholder: 'du@beispiel.de',
		interests: 'Was soll ein Helfer für dich tun? (optional)',
		interest: {
			customers: 'Kunden finden',
			competitors: 'Mitbewerber beobachten',
			social: 'Social-Beiträge schreiben',
			news: 'Neuigkeiten zusammenfassen',
			emails: 'E-Mails sortieren',
			events: 'Events planen',
			other: 'Etwas anderes'
		},
		wish: 'Was soll er tun?',
		wishPlaceholder: 'z. B. jede Woche die Preise meiner Lieferanten prüfen',
		button: 'Auf die Warteliste',
		working: 'Einen Moment…',
		consent:
			'Wir schicken dir einen Link zum Bestätigen. Danach schreiben wir dir, sobald deine Einladung bereit ist, und ab und zu, was es Neues bei Laamo gibt. Du kannst die Liste jederzeit verlassen.',
		sent: (email) => `Schau in dein Postfach: Wir haben einen Link an ${email} geschickt.`,
		sentHint: 'Tipp ihn an, um deinen Platz zu bestätigen. Nichts da? Schau im Spam-Ordner nach.',
		again: 'Andere Adresse verwenden',
		badEmail: 'Das sieht nicht wie eine E-Mail-Adresse aus.',
		slow: 'Gerade melden sich viele an. Bitte versuch es in ein paar Minuten noch einmal.',
		failed: 'Das hat nicht geklappt. Bitte versuch es in einer Minute noch einmal.',
		noScript: 'Das Anmeldeformular braucht JavaScript.'
	},
	problem: {
		title: 'Agenten sollten keinen Entwickler brauchen.',
		body: [
			'Die meisten Automatisierungs-Tools sehen aus wie ein Schaltplan: Kästen, Pfeile, Schlüssel, Fehlerprotokolle. Viele geben auf, bevor das erste Ergebnis da ist.',
			'Laamo sieht aus wie die Apps, die du jeden Tag nutzt. Ein Helfer ist ein kurzer Stapel bunter Bausteine, und jeder Baustein ist ein einfacher Satz.'
		],
		stackTitle: 'Ein ganzer Helfer:',
		stack: [
			{ group: 'when', text: 'Jeden Werktag um 7:30' },
			{ group: 'look', text: 'Nachrichten zu Bäckereien in Leipzig suchen' },
			{ group: 'think', text: 'Die 5 wichtigsten behalten' },
			{ group: 'think', text: 'In 5 Zeilen zusammenfassen' },
			{ group: 'do', text: 'In meinen Team-Chat posten' }
		]
	},
	how: {
		title: 'So funktioniert es',
		steps: [
			{
				title: 'Such dir einen Helfer aus',
				body: 'Nimm einen fertigen Helfer, oder beschreib in deinen Worten, was du brauchst.'
			},
			{
				title: 'Beantworte drei Fragen',
				body: 'Etwa deine Stadt, dein Geschäft und wo du Bescheid bekommst. Laamo merkt sich die Antworten für deine nächsten Helfer.'
			},
			{
				title: 'Ergebnisse landen in deinem Feed',
				body: 'Wisch durch, was er gefunden hat. Alles, was Laamo verlässt, wartet zuerst auf dein OK.'
			}
		],
		bricksTitle: 'Fünf Farben, fünf Fragen',
		bricks: [
			{ group: 'when', name: 'Wann', question: 'Wann soll er starten?' },
			{ group: 'look', name: 'Schauen', question: 'Was soll er lesen?' },
			{ group: 'think', name: 'Denken', question: 'Was soll er damit machen?' },
			{ group: 'do', name: 'Tun', question: 'Was soll sich ändern?' },
			{ group: 'ask', name: 'Frag mich', question: 'Wann braucht er dein OK?' }
		]
	},
	mobile: {
		title: 'Bau ihn im Bus.',
		body: 'Laamo funktioniert am Handy genauso wie am Computer. Richte einen Helfer im Browser ein, gib Ergebnisse mit einem Wisch frei und leg Laamo auf deinen Startbildschirm. Apps fürs Handy kommen bald.'
	},
	social: {
		title: 'Poste mit Posta auf deinen Social-Media-Konten.',
		soon: 'Bald',
		body: 'Ein Helfer schreibt den Beitrag und zeichnet ein Bild dazu. Du prüfst ihn, und Posta veröffentlicht ihn auf deinen Konten, sofort oder zu einer Zeit, die du wählst.',
		stack: [
			{ group: 'when', text: 'Jeden Dienstag und Freitag um 10:00' },
			{ group: 'look', text: 'Neuigkeiten aus meiner Branche suchen' },
			{ group: 'think', text: 'Einen Beitrag in meinem Ton schreiben' },
			{ group: 'think', text: 'Ein Bild dazu zeichnen' },
			{ group: 'ask', text: 'Erst prüfen' },
			{ group: 'do', text: 'Mit Posta um 12:00 posten' }
		]
	},
	customers: {
		title: 'Kunden finden. Mitbewerber im Blick behalten.',
		examples: [
			{
				name: 'Kundenfinder',
				body: 'Jeden Montag findet er Cafés in deiner Stadt, liest ihre Websites, behält die ohne Online-Buchung und schreibt für jedes eine erste E-Mail.',
				stack: [
					{ group: 'when', text: 'Jeden Montag um 8:00' },
					{ group: 'look', text: 'Cafés in Leipzig finden' },
					{ group: 'look', text: 'Ihre Websites lesen' },
					{ group: 'think', text: 'Die ohne Online-Buchung behalten' },
					{ group: 'ask', text: 'Lass mich wählen' },
					{ group: 'think', text: 'Eine kurze erste E-Mail schreiben' }
				]
			},
			{
				name: 'Mitbewerber-Recherche',
				body: 'Jede Woche sucht er, was deine Mitbewerber vorhaben, behält das Wichtige und schickt dir eine kurze Zusammenfassung.',
				stack: [
					{ group: 'when', text: 'Jeden Freitag um 9:00' },
					{ group: 'look', text: 'Neuigkeiten zu meinen Mitbewerbern suchen' },
					{ group: 'think', text: 'Preisänderungen und neue Angebote behalten' },
					{ group: 'think', text: 'Zusammenfassen' },
					{ group: 'do', text: 'In meinen Chat posten' }
				]
			}
		],
		note: 'Erste E-Mails bleiben Entwürfe. Du schickst die, die dir gefallen, aus deinem eigenen Postfach.'
	},
	fast: {
		title: 'Dein erstes Ergebnis in 5 Minuten.',
		steps: [
			{ time: '0:30', text: 'Mit deiner E-Mail anmelden' },
			{ time: '1:30', text: 'Einen Helfer aussuchen' },
			{ time: '2:30', text: 'Drei Fragen beantworten' },
			{ time: '5:00', text: 'Sein erstes Ergebnis ist in deinem Feed' }
		],
		note: 'Keine Einrichtungstermine, keine Schlüssel zum Kopieren, kein Handbuch.'
	},
	free: {
		title: 'Kein Lock-in.',
		points: [
			{ text: 'Lade jede Liste jederzeit als Tabellendatei (CSV) herunter.' },
			{ text: 'Deine Daten werden in der EU gespeichert.' },
			{ text: 'Ein KI-Modell ist dabei. Bring dein eigenes Modell mit, wenn du magst.', soon: true }
		]
	},
	soon: 'Bald',
	tryIt: {
		title: 'Probier jetzt einen Helfer aus.',
		body: 'Kein Konto, keine E-Mail. Such dir einen Beispiel-Helfer aus und sieh sein Ergebnis in unter einer Minute.',
		button: 'Jetzt ausprobieren'
	},
	faq: {
		title: 'Fragen',
		items: [
			{
				q: 'Muss ich programmieren können?',
				a: 'Nein. Du wählst Bausteine und beantwortest Fragen in einfachen Worten. Wer ein Handy bedienen kann, kann einen Helfer bauen.'
			},
			{
				q: 'Was kostet es?',
				a: 'Es wird einen kostenlosen Plan zum Einstieg geben. Die Preise nennen wir, bevor deine Einladung kommt.'
			},
			{
				q: 'Schickt ein Helfer etwas, ohne mich zu fragen?',
				a: 'Nur wenn du es so willst. Alles, was Laamo verlässt, wartet standardmäßig auf dein OK, und E-Mails an neue Kontakte sind immer Entwürfe, die du selbst schickst.'
			},
			{
				q: 'Wo sind meine Daten?',
				a: 'In der EU gespeichert. Dienste, die ein Helfer für eine Aufgabe nutzt, etwa eine Websuche oder ein KI-Modell, bekommen nur, was sie dafür brauchen.'
			},
			{
				q: 'Welche KI nutzt Laamo?',
				a: 'In jedem Plan ist ein KI-Modell enthalten, du brauchst also nirgends sonst ein Konto. Bald kannst du dein eigenes Modell mitbringen.'
			},
			{
				q: 'Gibt es eine App?',
				a: 'Laamo läuft im Browser auf Handy und Computer, und du kannst es auf deinen Startbildschirm legen. Apps fürs Handy kommen bald.'
			}
		]
	},
	end: {
		title: 'Hol dir deine Einladung.',
		body: 'Wir lassen nach und nach Leute rein. Trag dich ein, und wir schreiben dir, wenn du dran bist.'
	},
	confirm: {
		metaTitle: 'Platz bestätigen · Laamo',
		title: 'Ein Tap zum Bestätigen.',
		body: 'Bestätige, dass du auf die Laamo-Warteliste möchtest.',
		button: 'Ja, auf die Liste',
		working: 'Einen Moment…',
		doneTitle: 'Du stehst auf der Liste.',
		doneBody: 'Wir schreiben dir, sobald deine Einladung bereit ist.',
		shareTitle: 'Rück auf der Liste vor',
		shareBody:
			'Teile deinen Link. Jede Person, die sich damit einträgt, bringt dich weiter nach vorn.',
		copy: 'Link kopieren',
		copied: 'Kopiert',
		share: 'Teilen',
		badTitle: 'Dieser Link funktioniert nicht mehr.',
		badBody:
			'Vielleicht ist er älter als 7 Tage oder wurde schon benutzt. Trag dich noch einmal ein, dann schicken wir einen neuen.',
		home: 'Zurück zu Laamo',
		noLink: 'Diese Seite braucht den Link aus deiner E-Mail.'
	}
};

export const copy: Record<Lang, Copy> = { en, de };
