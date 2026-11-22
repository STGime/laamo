// Pages per kind of business (/for/<slug>): what Laamo's helpers do for a
// café, a trade business, a freelancer, a shop. People search for "AI for
// my kind of business", not for a product name. Each job links to its
// use-case page. Same copy rules as copy.ts.
import type { Lang } from '#lib/i18n/index.svelte.ts';

export interface IndustryText {
	/** Short name in lists and links ("Cafés and restaurants"). */
	name: string;
	/** The page's headline. */
	title: string;
	metaTitle: string;
	description: string;
	intro: string[];
	/** What helpers do here, each with the use case behind it. */
	jobs: { title: string; body: string; useCase: string }[];
	/** A week with the helpers, as a short list. */
	week: { when: string; what: string }[];
	faq: { q: string; a: string }[];
}

export interface Industry {
	slug: string;
	avatar: string;
	text: Record<Lang, IndustryText>;
}

export const INDUSTRIES: Industry[] = [
	{
		slug: 'cafes-restaurants',
		avatar: '☕',
		text: {
			en: {
				name: 'Cafés and restaurants',
				title: 'AI helpers for cafés and restaurants.',
				metaTitle: 'AI helpers for cafés and restaurants · Laamo',
				description:
					'What Laamo’s helpers do for a café or restaurant: tell you what the town is saying about you, what’s on this weekend, which emails need you today, and who nearby could order from you.',
				intro: [
					'You run a kitchen and a room full of guests. The work at the computer happens after closing, if at all.',
					'A Laamo helper does one of those small jobs for you, on a schedule, and puts the result on your phone. You set it up once in a few taps. No training, no software to learn.'
				],
				jobs: [
					{
						title: 'Hear what people write about you',
						body: 'Every Friday: new blog posts, forum threads and articles that name your place, summed up in a few lines.',
						useCase: 'mention-monitor'
					},
					{
						title: 'Know what’s on in town',
						body: 'A street festival, a fair, a match day: every morning the local news that change how many guests will come.',
						useCase: 'morning-briefing'
					},
					{
						title: 'Answer booking and catering emails faster',
						body: 'Forward the email, get a friendly reply draft with your prices and hours. You send it.',
						useCase: 'email-reply-drafts'
					},
					{
						title: 'Find offices and venues that could order from you',
						body: 'Every Monday: businesses nearby that could use your lunch delivery or catering, with a first email drafted for each.',
						useCase: 'customer-finder'
					}
				],
				week: [
					{
						when: 'Mon 8:00',
						what: '8 offices within walking distance, with a catering email drafted for each'
					},
					{
						when: 'Every day 7:30',
						what: 'Five lines of local news: the roadworks in your street start Wednesday'
					},
					{ when: 'When you forward one', what: 'A reply draft to the birthday party enquiry' },
					{ when: 'Fri 16:00', what: 'A city blog listed your breakfast. Here is what it says' }
				],
				faq: [
					{
						q: 'Does it take reservations or answer the phone?',
						a: 'No. Helpers read, search, sum up and write drafts. Bookings and calls stay with you and your booking system.'
					},
					{
						q: 'Do I need a computer?',
						a: 'No. You set helpers up and read their results on your phone.'
					},
					{
						q: 'Will it write to my guests by itself?',
						a: 'No. Anything meant for someone else is a draft that waits for your OK.'
					}
				]
			},
			de: {
				name: 'Cafés und Restaurants',
				title: 'KI-Helfer für Cafés und Restaurants.',
				metaTitle: 'KI-Helfer für Cafés, Restaurants und Gastronomie · Laamo',
				description:
					'Was Laamos Helfer für ein Café oder Restaurant tun: Sie sagen dir, was die Stadt über dich schreibt, was am Wochenende los ist, welche E-Mails dich heute brauchen und wer in der Nähe bei dir bestellen könnte.',
				intro: [
					'Du führst eine Küche und einen Raum voller Gäste. Die Arbeit am Computer passiert nach Ladenschluss, wenn überhaupt.',
					'Ein Laamo-Helfer nimmt dir eine dieser kleinen Aufgaben ab, nach Zeitplan, und legt das Ergebnis auf dein Handy. Du richtest ihn einmal mit ein paar Taps ein. Keine Schulung, keine Software zum Lernen.'
				],
				jobs: [
					{
						title: 'Hören, was über dich geschrieben wird',
						body: 'Jeden Freitag: neue Blogbeiträge, Forenthemen und Artikel, die dein Lokal nennen, in ein paar Zeilen zusammengefasst.',
						useCase: 'mention-monitor'
					},
					{
						title: 'Wissen, was in der Stadt los ist',
						body: 'Ein Straßenfest, eine Messe, ein Spieltag: jeden Morgen die lokalen Nachrichten, die ändern, wie viele Gäste kommen.',
						useCase: 'morning-briefing'
					},
					{
						title: 'Reservierungs- und Catering-Mails schneller beantworten',
						body: 'Leite die E-Mail weiter und bekomm einen freundlichen Antwortentwurf mit deinen Preisen und Zeiten. Du schickst ihn ab.',
						useCase: 'email-reply-drafts'
					},
					{
						title: 'Büros und Veranstaltungsorte finden, die bei dir bestellen könnten',
						body: 'Jeden Montag: Betriebe in der Nähe, die deinen Mittagstisch oder dein Catering brauchen könnten, mit einer ersten E-Mail als Entwurf.',
						useCase: 'customer-finder'
					}
				],
				week: [
					{ when: 'Mo 8:00', what: '8 Büros in Laufweite, mit einem Catering-Entwurf für jedes' },
					{
						when: 'Täglich 7:30',
						what: 'Fünf Zeilen Lokalnachrichten: Die Baustelle in deiner Straße beginnt Mittwoch'
					},
					{
						when: 'Wenn du eine weiterleitest',
						what: 'Ein Antwortentwurf auf die Anfrage zur Geburtstagsfeier'
					},
					{ when: 'Fr 16:00', what: 'Ein Stadtblog hat dein Frühstück empfohlen. Das steht drin' }
				],
				faq: [
					{
						q: 'Nimmt er Reservierungen an oder geht ans Telefon?',
						a: 'Nein. Helfer lesen, suchen, fassen zusammen und schreiben Entwürfe. Reservierungen und Anrufe bleiben bei dir und deinem Buchungssystem.'
					},
					{
						q: 'Brauche ich einen Computer?',
						a: 'Nein. Du richtest Helfer am Handy ein und liest dort ihre Ergebnisse.'
					},
					{
						q: 'Schreibt er meinen Gästen von allein?',
						a: 'Nein. Alles, was für jemand anderen gedacht ist, ist ein Entwurf und wartet auf dein OK.'
					}
				]
			}
		}
	},
	{
		slug: 'trades',
		avatar: '🔧',
		text: {
			en: {
				name: 'Trades and crafts',
				title: 'AI helpers for trade and craft businesses.',
				metaTitle: 'AI helpers for tradespeople and craft businesses · Laamo',
				description:
					'What Laamo’s helpers do for electricians, painters, joiners and other trade businesses: find public tenders, answer enquiries, find property managers to work for, and hear about trade fairs in time.',
				intro: [
					'You are on a building site all day. Tenders, enquiries and the search for the next steady client wait until the evening.',
					'A Laamo helper does one of those jobs for you every week and puts the result on your phone. You set it up once, in plain words.'
				],
				jobs: [
					{
						title: 'Find public tenders in your trade',
						body: 'Every Monday: new tenders and funding programmes in your trade and region, with their deadlines in one list.',
						useCase: 'tender-finder'
					},
					{
						title: 'Answer enquiries the same day',
						body: 'Forward a customer’s email from the van. The helper drafts a clear, friendly reply for you to send.',
						useCase: 'email-reply-drafts'
					},
					{
						title: 'Find property managers and builders who need you',
						body: 'A list of property managers, housing companies or architects in your area, with a first email drafted for each.',
						useCase: 'customer-finder'
					},
					{
						title: 'Meet the people who place the orders',
						body: 'Every week: newly announced trade fairs, building days and chamber evenings within reach.',
						useCase: 'trade-fair-finder'
					}
				],
				week: [
					{
						when: 'Mon 7:00',
						what: '3 new tenders for painting work, bids due in December and January'
					},
					{ when: 'Mon 8:00', what: '8 property managers in your district, with a draft for each' },
					{
						when: 'When you forward one',
						what: 'A reply draft to the bathroom renovation enquiry'
					},
					{ when: 'Mon 8:00', what: 'A regional building fair in March: keep or skip' }
				],
				faq: [
					{
						q: 'Does it write quotes or invoices?',
						a: 'No. It finds work and drafts messages. Quotes and invoices stay in the program you use for them.'
					},
					{
						q: 'I’m not good with computers. Can I use it?',
						a: 'Yes. You pick a helper and answer up to three questions in your own words. That is the whole setup.'
					},
					{
						q: 'Which tender portals does it read?',
						a: 'It searches the open web, so it finds notices that are published without a login. Portals that need an account stay closed to it.'
					}
				]
			},
			de: {
				name: 'Handwerk',
				title: 'KI-Helfer für Handwerksbetriebe.',
				metaTitle: 'KI für Handwerker und Handwerksbetriebe · Laamo',
				description:
					'Was Laamos Helfer für Elektriker, Maler, Tischler und andere Handwerksbetriebe tun: öffentliche Ausschreibungen finden, Anfragen beantworten, Hausverwaltungen als Auftraggeber finden und rechtzeitig von Messen erfahren.',
				intro: [
					'Du bist den ganzen Tag auf der Baustelle. Ausschreibungen, Anfragen und die Suche nach dem nächsten festen Auftraggeber warten bis zum Abend.',
					'Ein Laamo-Helfer nimmt dir jede Woche eine dieser Aufgaben ab und legt das Ergebnis auf dein Handy. Du richtest ihn einmal ein, in einfachen Worten.'
				],
				jobs: [
					{
						title: 'Öffentliche Ausschreibungen für dein Gewerk finden',
						body: 'Jeden Montag: neue Ausschreibungen und Förderprogramme für dein Gewerk und deine Region, mit Fristen in einer Liste.',
						useCase: 'tender-finder'
					},
					{
						title: 'Anfragen noch am selben Tag beantworten',
						body: 'Leite die E-Mail eines Kunden vom Transporter aus weiter. Der Helfer entwirft eine klare, freundliche Antwort, die du abschickst.',
						useCase: 'email-reply-drafts'
					},
					{
						title: 'Hausverwaltungen und Bauträger finden, die dich brauchen',
						body: 'Eine Liste von Hausverwaltungen, Wohnungsgesellschaften oder Architekten in deiner Gegend, mit einer ersten E-Mail als Entwurf.',
						useCase: 'customer-finder'
					},
					{
						title: 'Die Leute treffen, die Aufträge vergeben',
						body: 'Jede Woche: neu angekündigte Messen, Bautage und Abende der Handwerkskammer in Reichweite.',
						useCase: 'trade-fair-finder'
					}
				],
				week: [
					{
						when: 'Mo 7:00',
						what: '3 neue Ausschreibungen für Malerarbeiten, Fristen im Dezember und Januar'
					},
					{
						when: 'Mo 8:00',
						what: '8 Hausverwaltungen in deinem Bezirk, mit einem Entwurf für jede'
					},
					{
						when: 'Wenn du eine weiterleitest',
						what: 'Ein Antwortentwurf auf die Anfrage zur Badsanierung'
					},
					{ when: 'Mo 8:00', what: 'Eine regionale Baumesse im März: behalten oder überspringen' }
				],
				faq: [
					{
						q: 'Schreibt er Angebote oder Rechnungen?',
						a: 'Nein. Er findet Aufträge und entwirft Nachrichten. Angebote und Rechnungen bleiben in dem Programm, das du dafür nutzt.'
					},
					{
						q: 'Ich kann nicht gut mit Computern. Komme ich damit zurecht?',
						a: 'Ja. Du suchst dir einen Helfer aus und beantwortest bis zu drei Fragen in deinen eigenen Worten. Das ist die ganze Einrichtung.'
					},
					{
						q: 'Welche Vergabeportale liest er?',
						a: 'Er sucht im offenen Web und findet daher Bekanntmachungen, die ohne Anmeldung veröffentlicht sind. Portale mit Konto bleiben ihm verschlossen.'
					}
				]
			}
		}
	},
	{
		slug: 'freelancers',
		avatar: '💼',
		text: {
			en: {
				name: 'Freelancers and consultants',
				title: 'AI helpers for freelancers and the self-employed.',
				metaTitle: 'AI helpers for freelancers and the self-employed · Laamo',
				description:
					'What Laamo’s helpers do when you work alone: find businesses that could hire you, keep an eye on your field, draft replies to enquiries and find events worth going to.',
				intro: [
					'When you work alone, you are also the sales department, the press office and the back office. Those jobs get the hours that are left over.',
					'A Laamo helper takes one of them: it runs on a schedule, does the searching and reading, and shows you only what needs a decision.'
				],
				jobs: [
					{
						title: 'Find businesses that could hire you',
						body: 'Every Monday: agencies, studios or firms in your region that fit what you offer, with a first email drafted for each.',
						useCase: 'customer-finder'
					},
					{
						title: 'Build your own contact list',
						body: 'All the architects in three towns, with email and phone, in a list you can download as a spreadsheet.',
						useCase: 'business-list'
					},
					{
						title: 'Stay on top of your field',
						body: 'Every weekday morning: the five pieces of news that matter for your work, in a few lines.',
						useCase: 'morning-briefing'
					},
					{
						title: 'Reply to enquiries without losing the morning',
						body: 'Forward the email. The helper drafts the reply; you adjust the price and send it.',
						useCase: 'email-reply-drafts'
					}
				],
				week: [
					{
						when: 'Mon 8:00',
						what: '8 design studios in Hamburg that have no in-house illustrator'
					},
					{
						when: 'Weekdays 7:30',
						what: 'Five lines on your field: a new funding round for creative work opens'
					},
					{ when: 'When you forward one', what: 'A reply draft to the workshop enquiry' },
					{ when: 'When you tap Go', what: '42 architects in Bremen saved to your list' }
				],
				faq: [
					{
						q: 'Does it send cold emails for me?',
						a: 'No. First emails are drafts. You read them, change them and send them from your own mailbox.'
					},
					{
						q: 'Can I build a helper for something that isn’t listed?',
						a: 'Yes. A helper is a short stack of bricks: when it runs, where it looks, what it keeps, what it does. You can build your own in a few minutes.'
					},
					{
						q: 'What does it cost?',
						a: 'There will be a free plan to start with. Prices are shared before your invite arrives.'
					}
				]
			},
			de: {
				name: 'Selbstständige und Freiberufler',
				title: 'KI-Helfer für Selbstständige und Freiberufler.',
				metaTitle: 'KI-Helfer für Selbstständige und Freiberufler · Laamo',
				description:
					'Was Laamos Helfer tun, wenn du allein arbeitest: Betriebe finden, die dich beauftragen könnten, dein Fachgebiet im Blick behalten, Antworten auf Anfragen entwerfen und Veranstaltungen finden, die sich lohnen.',
				intro: [
					'Wer allein arbeitet, ist auch Vertrieb, Pressestelle und Büro. Für diese Aufgaben bleiben die Stunden, die übrig sind.',
					'Ein Laamo-Helfer übernimmt eine davon: Er läuft nach Zeitplan, sucht und liest für dich und zeigt dir nur, was eine Entscheidung braucht.'
				],
				jobs: [
					{
						title: 'Betriebe finden, die dich beauftragen könnten',
						body: 'Jeden Montag: Agenturen, Studios oder Firmen in deiner Region, die zu deinem Angebot passen, mit einer ersten E-Mail als Entwurf.',
						useCase: 'customer-finder'
					},
					{
						title: 'Deine eigene Kontaktliste aufbauen',
						body: 'Alle Architekturbüros in drei Städten, mit E-Mail und Telefon, in einer Liste, die du als Tabelle herunterladen kannst.',
						useCase: 'business-list'
					},
					{
						title: 'In deinem Fachgebiet auf dem Laufenden bleiben',
						body: 'Jeden Werktag am Morgen: die fünf Neuigkeiten, die für deine Arbeit zählen, in ein paar Zeilen.',
						useCase: 'morning-briefing'
					},
					{
						title: 'Anfragen beantworten, ohne den Vormittag zu verlieren',
						body: 'Leite die E-Mail weiter. Der Helfer entwirft die Antwort; du passt den Preis an und schickst sie ab.',
						useCase: 'email-reply-drafts'
					}
				],
				week: [
					{ when: 'Mo 8:00', what: '8 Designstudios in Hamburg ohne eigene Illustratorin' },
					{
						when: 'Werktags 7:30',
						what: 'Fünf Zeilen zu deinem Fachgebiet: Eine neue Förderrunde für Kreativarbeit startet'
					},
					{
						when: 'Wenn du eine weiterleitest',
						what: 'Ein Antwortentwurf auf die Workshop-Anfrage'
					},
					{
						when: 'Wenn du auf Los tippst',
						what: '42 Architekturbüros in Bremen in deiner Liste gespeichert'
					}
				],
				faq: [
					{
						q: 'Verschickt er Kaltakquise-Mails für mich?',
						a: 'Nein. Erste E-Mails sind Entwürfe. Du liest sie, änderst sie und schickst sie aus deinem eigenen Postfach.'
					},
					{
						q: 'Kann ich einen Helfer für etwas bauen, das hier nicht steht?',
						a: 'Ja. Ein Helfer ist ein kurzer Stapel Bausteine: wann er läuft, wo er nachsieht, was er behält, was er tut. Deinen eigenen baust du in ein paar Minuten.'
					},
					{
						q: 'Was kostet das?',
						a: 'Zum Start wird es einen kostenlosen Tarif geben. Die Preise erfährst du, bevor deine Einladung kommt.'
					}
				]
			}
		}
	},
	{
		slug: 'shops',
		avatar: '🛍️',
		text: {
			en: {
				name: 'Shops and online sellers',
				title: 'AI helpers for small shops and online sellers.',
				metaTitle: 'AI helpers for small shops and online sellers · Laamo',
				description:
					'What Laamo’s helpers do for a shop: check what competitors charge, tell you what customers write about you, answer product questions and find markets to sell at.',
				intro: [
					'A small shop competes with sellers who have a department for everything. You have the hour before opening.',
					'A Laamo helper uses that hour for you. It checks, reads and sums up on a schedule, and shows you the result as a short card on your phone.'
				],
				jobs: [
					{
						title: 'Check competitors’ prices every week',
						body: 'Name the product pages once. Every Monday the current prices are in one list, ready to compare.',
						useCase: 'price-check'
					},
					{
						title: 'Read what customers write about your shop',
						body: 'Every Friday: new posts, articles and reviews on the open web that name your shop.',
						useCase: 'mention-monitor'
					},
					{
						title: 'Answer product questions in your tone',
						body: 'Forward the customer’s email and get a reply draft with the facts about your shop. You send it.',
						useCase: 'email-reply-drafts'
					},
					{
						title: 'Find markets and fairs to sell at',
						body: 'Every week: newly announced markets, pop-up events and trade fairs in your region. Swipe to keep.',
						useCase: 'trade-fair-finder'
					}
				],
				week: [
					{ when: 'Mon 6:00', what: '12 prices read: one competitor dropped oat milk by 10 cents' },
					{
						when: 'Mon 8:00',
						what: 'A makers’ market in May is taking applications: keep or skip'
					},
					{ when: 'When you forward one', what: 'A reply draft to “Do you ship to Austria?”' },
					{ when: 'Fri 16:00', what: 'A neighbourhood forum recommends your shop for gifts' }
				],
				faq: [
					{
						q: 'Does it connect to my shop system?',
						a: 'No. Helpers work with what is public on the web, with emails you forward, and with lists you keep in Laamo.'
					},
					{
						q: 'Can it read prices on any website?',
						a: 'On pages that show the price when they open. Prices behind a login or a size choice stay hidden from it.'
					},
					{
						q: 'Can it post to my social accounts?',
						a: 'Soon. A helper that writes a post, draws a picture for it and publishes after your OK is being built.'
					}
				]
			},
			de: {
				name: 'Läden und Online-Händler',
				title: 'KI-Helfer für kleine Läden und Online-Händler.',
				metaTitle: 'KI-Helfer für Einzelhandel und kleine Online-Shops · Laamo',
				description:
					'Was Laamos Helfer für einen Laden tun: prüfen, was die Konkurrenz verlangt, dir sagen, was Kunden über dich schreiben, Produktfragen beantworten und Märkte finden, auf denen du verkaufen kannst.',
				intro: [
					'Ein kleiner Laden konkurriert mit Händlern, die für alles eine Abteilung haben. Du hast die Stunde vor Ladenöffnung.',
					'Ein Laamo-Helfer nutzt diese Stunde für dich. Er prüft, liest und fasst nach Zeitplan zusammen und zeigt dir das Ergebnis als kurze Karte auf deinem Handy.'
				],
				jobs: [
					{
						title: 'Die Preise der Konkurrenz jede Woche prüfen',
						body: 'Nenn die Produktseiten einmal. Jeden Montag stehen die aktuellen Preise in einer Liste, bereit zum Vergleichen.',
						useCase: 'price-check'
					},
					{
						title: 'Lesen, was Kunden über deinen Laden schreiben',
						body: 'Jeden Freitag: neue Beiträge, Artikel und Bewertungen im offenen Web, die deinen Laden nennen.',
						useCase: 'mention-monitor'
					},
					{
						title: 'Produktfragen in deinem Ton beantworten',
						body: 'Leite die E-Mail der Kundin weiter und bekomm einen Antwortentwurf mit den Fakten zu deinem Laden. Du schickst ihn ab.',
						useCase: 'email-reply-drafts'
					},
					{
						title: 'Märkte und Messen finden, auf denen du verkaufen kannst',
						body: 'Jede Woche: neu angekündigte Märkte, Pop-up-Events und Messen in deiner Region. Wisch, um sie zu behalten.',
						useCase: 'trade-fair-finder'
					}
				],
				week: [
					{
						when: 'Mo 6:00',
						what: '12 Preise gelesen: Ein Mitbewerber hat Haferdrink um 10 Cent gesenkt'
					},
					{
						when: 'Mo 8:00',
						what: 'Ein Kunsthandwerkermarkt im Mai nimmt Bewerbungen an: behalten oder überspringen'
					},
					{
						when: 'Wenn du eine weiterleitest',
						what: 'Ein Antwortentwurf auf „Liefert ihr nach Österreich?“'
					},
					{ when: 'Fr 16:00', what: 'Ein Stadtteilforum empfiehlt deinen Laden für Geschenke' }
				],
				faq: [
					{
						q: 'Verbindet er sich mit meinem Shopsystem?',
						a: 'Nein. Helfer arbeiten mit dem, was öffentlich im Web steht, mit E-Mails, die du weiterleitest, und mit Listen, die du in Laamo führst.'
					},
					{
						q: 'Kann er Preise auf jeder Website lesen?',
						a: 'Auf Seiten, die den Preis beim Öffnen zeigen. Preise hinter einer Anmeldung oder einer Größenauswahl bleiben ihm verborgen.'
					},
					{
						q: 'Kann er auf meinen Social-Media-Konten posten?',
						a: 'Bald. Ein Helfer, der einen Beitrag schreibt, ein Bild dazu malt und nach deinem OK veröffentlicht, ist in Arbeit.'
					}
				]
			}
		}
	}
];

export function industry(slug: string): Industry | undefined {
	return INDUSTRIES.find((i) => i.slug === slug);
}

/** Labels for the industry pages, per language. */
export const INDUSTRY_UI: Record<
	Lang,
	{ listTitle: string; jobs: string; week: string; weekLead: string; more: string; faq: string }
> = {
	en: {
		listTitle: 'For your kind of business',
		jobs: 'What helpers do for you',
		week: 'A week with your helpers',
		weekLead: 'What lands in your Feed, without you asking:',
		more: 'How this helper works',
		faq: 'Questions'
	},
	de: {
		listTitle: 'Für deine Art von Betrieb',
		jobs: 'Was Helfer für dich tun',
		week: 'Eine Woche mit deinen Helfern',
		weekLead: 'Was in deinem Feed landet, ohne dass du fragst:',
		more: 'So arbeitet dieser Helfer',
		faq: 'Fragen'
	}
};
