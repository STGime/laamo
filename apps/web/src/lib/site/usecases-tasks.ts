// More use-case pages, each named after a task people search for ("list of
// businesses with emails", "find public tenders", "check competitor
// prices"). Every one is built from bricks that run today; what a helper
// can't do is said on the page. Same copy rules as copy.ts.
import type { UseCase } from './usecases.ts';

export const TASK_USE_CASES: UseCase[] = [
	{
		slug: 'business-list',
		avatar: '📋',
		text: {
			en: {
				name: 'Business List Builder',
				title: 'Build a list of local businesses, with email and phone.',
				metaTitle: 'Build a list of local businesses with emails · Laamo',
				description:
					'A helper that finds every business of one kind in a city or region, reads their websites and saves name, address, website, email and phone to a list you can download as a spreadsheet.',
				problem:
					'You need a list of all the dentists, cafés or roofers in your area. Building it by hand means a map in one tab, forty websites in the others and an afternoon of copying.',
				how: 'Tell the helper what kind of business and where. It looks them up on the open map of the world, opens each website and its contact page, and pulls out the email address and phone number. Every business becomes a row in your list. Run it again for the next town: businesses already in the list are skipped.',
				stack: [
					{ group: 'when', text: 'When I tap Go, ask “Which town?”' },
					{ group: 'look', text: 'Find dentists there' },
					{ group: 'look', text: 'Read their website and contact page' },
					{ group: 'think', text: 'Pull out email and phone' },
					{ group: 'do', text: 'Save new ones to my Dentists list' }
				],
				result: {
					title: 'Saved 42 dentists in Hanover',
					lines: [
						'Zahnarztpraxis am Maschsee: address, website, phone, email',
						'Dr. Brandt & Kollegen: address, website, phone (no email on the site)',
						'40 more rows in your Dentists list'
					]
				},
				questions: [
					'What kind of business should I look for?',
					'Where? A town, a region or a country.'
				],
				notes: [
					'An email or phone number is only saved when it is really on the business’s own website. The helper never makes one up.',
					'These are public business contacts. Whether you may write to them unasked depends on your country’s rules.'
				],
				faq: [
					{
						q: 'Where does the data come from?',
						a: 'Names and addresses come from OpenStreetMap, the open map anyone can improve. Email addresses and phone numbers come from each business’s own website.'
					},
					{
						q: 'Can I get the list as a spreadsheet?',
						a: 'Yes. Every list downloads as a CSV file, which opens in any spreadsheet program.'
					},
					{
						q: 'How many businesses does one run find?',
						a: 'Up to 50 per run. For a whole region, run it town by town: the list keeps growing and never holds the same business twice.'
					}
				]
			},
			de: {
				name: 'Firmenlisten-Helfer',
				title: 'Eine Liste lokaler Betriebe aufbauen, mit E-Mail und Telefon.',
				metaTitle: 'Firmenliste mit E-Mail-Adressen erstellen · Laamo',
				description:
					'Ein Helfer, der alle Betriebe einer Art in einer Stadt oder Region findet, ihre Websites liest und Name, Adresse, Website, E-Mail und Telefon in einer Liste speichert, die du als Tabelle herunterladen kannst.',
				problem:
					'Du brauchst eine Liste aller Zahnärzte, Cafés oder Dachdecker in deiner Gegend. Von Hand heißt das: eine Karte in einem Tab, vierzig Websites in den anderen und ein Nachmittag Kopieren.',
				how: 'Sag dem Helfer, welche Art von Betrieb und wo. Er schlägt sie auf der offenen Weltkarte nach, öffnet jede Website samt Kontaktseite und zieht E-Mail-Adresse und Telefonnummer heraus. Jeder Betrieb wird eine Zeile in deiner Liste. Lass ihn für den nächsten Ort noch einmal laufen: Betriebe, die schon in der Liste stehen, überspringt er.',
				stack: [
					{ group: 'when', text: 'Wenn ich auf Los tippe, frag „Welcher Ort?“' },
					{ group: 'look', text: 'Zahnärzte dort finden' },
					{ group: 'look', text: 'Ihre Website und Kontaktseite lesen' },
					{ group: 'think', text: 'E-Mail und Telefon herausziehen' },
					{ group: 'do', text: 'Neue in meiner Zahnärzte-Liste speichern' }
				],
				result: {
					title: '42 Zahnärzte in Hannover gespeichert',
					lines: [
						'Zahnarztpraxis am Maschsee: Adresse, Website, Telefon, E-Mail',
						'Dr. Brandt & Kollegen: Adresse, Website, Telefon (keine E-Mail auf der Seite)',
						'40 weitere Zeilen in deiner Zahnärzte-Liste'
					]
				},
				questions: [
					'Nach welcher Art von Betrieb soll ich suchen?',
					'Wo? Ein Ort, eine Region oder ein Land.'
				],
				notes: [
					'Eine E-Mail-Adresse oder Telefonnummer wird nur gespeichert, wenn sie wirklich auf der Website des Betriebs steht. Der Helfer denkt sich nie eine aus.',
					'Das sind öffentliche Geschäftskontakte. Ob du sie ungefragt anschreiben darfst, hängt von den Regeln deines Landes ab.'
				],
				faq: [
					{
						q: 'Woher kommen die Daten?',
						a: 'Namen und Adressen kommen aus OpenStreetMap, der offenen Karte, an der alle mitarbeiten können. E-Mail-Adressen und Telefonnummern kommen von der Website des jeweiligen Betriebs.'
					},
					{
						q: 'Bekomme ich die Liste als Tabelle?',
						a: 'Ja. Jede Liste lässt sich als CSV-Datei herunterladen, die jedes Tabellenprogramm öffnet.'
					},
					{
						q: 'Wie viele Betriebe findet ein Lauf?',
						a: 'Bis zu 50 pro Lauf. Für eine ganze Region lässt du ihn Ort für Ort laufen: Die Liste wächst weiter und enthält nie denselben Betrieb zweimal.'
					}
				]
			}
		}
	},
	{
		slug: 'tender-finder',
		avatar: '🏛️',
		text: {
			en: {
				name: 'Tender Finder',
				title: 'Find public tenders and grants that fit your business, every week.',
				metaTitle: 'Find public tenders and grants for small businesses · Laamo',
				description:
					'A helper that searches the web every week for public tenders, calls and grants in your field and region, keeps the ones that fit and saves them to a list with their deadlines.',
				problem:
					'Public contracts and grants are published on dozens of sites. Small businesses miss most of them, because nobody has the time to check them all every week.',
				how: 'Every Monday the helper searches for new tenders, calls for bids and funding programmes in your field and your region. It keeps the ones a business like yours could apply for, pulls out the deadline and saves each to your list with a link to the original notice. You get a short summary in your chat.',
				stack: [
					{ group: 'when', text: 'Every Monday at 7:00' },
					{ group: 'look', text: 'Search tenders for painting work in Saxony' },
					{ group: 'think', text: 'Keep the ones a small painting business can apply for' },
					{ group: 'think', text: 'Pull out title and deadline' },
					{ group: 'do', text: 'Save new ones to my Tenders list' },
					{ group: 'think', text: 'Sum them up in a few lines' },
					{ group: 'do', text: 'Post it to my chat' }
				],
				result: {
					title: '3 new tenders this week',
					lines: [
						'Repainting of a primary school, Dresden: bids until 12 December',
						'Facade work, housing cooperative, Chemnitz: bids until 5 January',
						'Funding for apprentice training in crafts: apply until 31 January'
					]
				},
				questions: [
					'What does your business do? (asked once, remembered for all helpers)',
					'Which region should I watch?',
					'Where should I send the summary?'
				],
				notes: [
					'The helper finds what is published on the open web. Portals that need a login stay closed to it.',
					'Always read the original notice before you apply: the helper’s summary is a pointer, not the terms.'
				],
				faq: [
					{
						q: 'Does it also find grants and funding programmes?',
						a: 'Yes. You say in plain words what to look for: tenders, calls for bids, grants or all of them.'
					},
					{
						q: 'Will I see the same tender twice?',
						a: 'No. Each tender is saved to your list once, and only new ones are in the weekly summary.'
					}
				]
			},
			de: {
				name: 'Ausschreibungs-Finder',
				title: 'Öffentliche Ausschreibungen und Fördermittel finden, die zu deinem Betrieb passen.',
				metaTitle: 'Öffentliche Ausschreibungen und Fördermittel finden · Laamo',
				description:
					'Ein Helfer, der jede Woche im Web nach öffentlichen Ausschreibungen, Aufrufen und Förderprogrammen in deinem Bereich und deiner Region sucht, die passenden behält und sie mit Frist in einer Liste speichert.',
				problem:
					'Öffentliche Aufträge und Fördermittel werden auf Dutzenden Seiten veröffentlicht. Kleine Betriebe verpassen die meisten, weil niemand die Zeit hat, jede Woche alle durchzusehen.',
				how: 'Jeden Montag sucht der Helfer nach neuen Ausschreibungen, Vergaben und Förderprogrammen in deinem Bereich und deiner Region. Er behält die, auf die sich ein Betrieb wie deiner bewerben kann, zieht die Frist heraus und speichert jede mit Link zur Originalbekanntmachung in deiner Liste. Du bekommst eine kurze Zusammenfassung in deinen Chat.',
				stack: [
					{ group: 'when', text: 'Jeden Montag um 7:00' },
					{ group: 'look', text: 'Ausschreibungen für Malerarbeiten in Sachsen suchen' },
					{
						group: 'think',
						text: 'Die behalten, auf die sich ein kleiner Malerbetrieb bewerben kann'
					},
					{ group: 'think', text: 'Titel und Frist herausziehen' },
					{ group: 'do', text: 'Neue in meiner Ausschreibungen-Liste speichern' },
					{ group: 'think', text: 'In ein paar Zeilen zusammenfassen' },
					{ group: 'do', text: 'In meinen Chat posten' }
				],
				result: {
					title: '3 neue Ausschreibungen diese Woche',
					lines: [
						'Neuanstrich einer Grundschule, Dresden: Angebote bis 12. Dezember',
						'Fassadenarbeiten, Wohnungsgenossenschaft, Chemnitz: Angebote bis 5. Januar',
						'Förderung für Ausbildung im Handwerk: Antrag bis 31. Januar'
					]
				},
				questions: [
					'Was macht dein Betrieb? (einmal gefragt, für alle Helfer gemerkt)',
					'Welche Region soll ich beobachten?',
					'Wohin soll die Zusammenfassung?'
				],
				notes: [
					'Der Helfer findet, was im offenen Web veröffentlicht ist. Portale mit Anmeldung bleiben ihm verschlossen.',
					'Lies immer die Originalbekanntmachung, bevor du dich bewirbst: Die Zusammenfassung des Helfers ist ein Hinweis, nicht die Bedingungen.'
				],
				faq: [
					{
						q: 'Findet er auch Fördermittel und Förderprogramme?',
						a: 'Ja. Du sagst in einfachen Worten, wonach er suchen soll: Ausschreibungen, Vergaben, Fördermittel oder alles zusammen.'
					},
					{
						q: 'Sehe ich dieselbe Ausschreibung zweimal?',
						a: 'Nein. Jede Ausschreibung wird einmal in deiner Liste gespeichert, und nur neue stehen in der wöchentlichen Zusammenfassung.'
					}
				]
			}
		}
	},
	{
		slug: 'mention-monitor',
		avatar: '💬',
		text: {
			en: {
				name: 'Mention Watch',
				title: 'Know when someone writes about your business.',
				metaTitle: 'Monitor mentions and reviews of your business · Laamo',
				description:
					'A helper that searches the web every week for new articles, posts and reviews that name your business, and sums up what people say in a few lines in your chat.',
				problem:
					'A local blog reviews your shop, a forum thread complains about your delivery, the paper mentions your opening. You hear about it weeks later, from a customer.',
				how: 'Every Friday the helper searches the web for your business name together with your town. It keeps what is really about you, saves each page to a list so you only hear of it once, sums up what was said and whether it was friendly, and posts it to your chat with the links. So you can say thank you, or put something right, while it still matters.',
				stack: [
					{ group: 'when', text: 'Every Friday at 16:00' },
					{ group: 'look', text: 'Search “Café Sonne” Leipzig' },
					{ group: 'think', text: 'Keep the pages that are about my café' },
					{ group: 'do', text: 'Save new ones to my Mentions list' },
					{ group: 'think', text: 'Sum up what they say' },
					{ group: 'do', text: 'Post it to my chat' }
				],
				result: {
					title: 'Café Sonne on the web this week',
					lines: [
						'A city blog put you in its “10 best breakfasts” and praises the cheesecake.',
						'A neighbourhood forum asks whether you still open on Mondays. Nobody has answered yet.',
						'Nothing new in the local paper.'
					]
				},
				questions: [
					'What is your business called?',
					'Which town is it in?',
					'Where should I tell you?'
				],
				notes: [
					'The helper sees what a web search finds. Reviews inside apps, or on pages that need a login, are out of its reach.',
					'A common name finds namesakes. Adding the town, or a word like “bakery”, keeps the results about you.'
				],
				faq: [
					{
						q: 'Can it watch my competitors too?',
						a: 'Yes. Name a competitor instead of your own business, or use the competitor research helper, which looks for price changes and new offers.'
					},
					{
						q: 'How often does it look?',
						a: 'As often as you set: every day, every week, or when you tap Go.'
					}
				]
			},
			de: {
				name: 'Erwähnungs-Wächter',
				title: 'Erfahre, wenn jemand über dein Geschäft schreibt.',
				metaTitle: 'Erwähnungen und Bewertungen deines Geschäfts beobachten · Laamo',
				description:
					'Ein Helfer, der jede Woche im Web nach neuen Artikeln, Beiträgen und Bewertungen sucht, die dein Geschäft nennen, und dir in ein paar Zeilen im Chat zusammenfasst, was die Leute sagen.',
				problem:
					'Ein Stadtblog bespricht deinen Laden, in einem Forum beschwert sich jemand über die Lieferung, die Zeitung erwähnt deine Eröffnung. Du erfährst es Wochen später, von einer Kundin.',
				how: 'Jeden Freitag sucht der Helfer im Web nach dem Namen deines Geschäfts zusammen mit deinem Ort. Er behält, was wirklich von dir handelt, speichert jede Seite in einer Liste, damit du nur einmal davon hörst, fasst zusammen, was gesagt wurde und ob es freundlich war, und postet es mit den Links in deinen Chat. So kannst du dich bedanken oder etwas geraderücken, solange es noch zählt.',
				stack: [
					{ group: 'when', text: 'Jeden Freitag um 16:00' },
					{ group: 'look', text: '„Café Sonne“ Leipzig suchen' },
					{ group: 'think', text: 'Die Seiten behalten, die von meinem Café handeln' },
					{ group: 'do', text: 'Neue in meiner Erwähnungen-Liste speichern' },
					{ group: 'think', text: 'Zusammenfassen, was sie sagen' },
					{ group: 'do', text: 'In meinen Chat posten' }
				],
				result: {
					title: 'Café Sonne im Web diese Woche',
					lines: [
						'Ein Stadtblog führt dich unter den „10 besten Frühstücken“ und lobt den Käsekuchen.',
						'In einem Stadtteilforum fragt jemand, ob du montags noch offen hast. Bisher hat niemand geantwortet.',
						'Nichts Neues in der Lokalzeitung.'
					]
				},
				questions: [
					'Wie heißt dein Geschäft?',
					'In welchem Ort ist es?',
					'Wo soll ich dir Bescheid geben?'
				],
				notes: [
					'Der Helfer sieht, was eine Websuche findet. Bewertungen in Apps oder auf Seiten mit Anmeldung erreicht er nicht.',
					'Ein häufiger Name findet Namensvettern. Mit dem Ort oder einem Wort wie „Bäckerei“ dazu bleiben die Ergebnisse bei dir.'
				],
				faq: [
					{
						q: 'Kann er auch meine Mitbewerber beobachten?',
						a: 'Ja. Nenn statt deines Geschäfts einen Mitbewerber, oder nimm den Helfer für Mitbewerber-Recherche, der nach Preisänderungen und neuen Angeboten sucht.'
					},
					{
						q: 'Wie oft schaut er nach?',
						a: 'So oft du es einstellst: jeden Tag, jede Woche oder wenn du auf Los tippst.'
					}
				]
			}
		}
	},
	{
		slug: 'price-check',
		avatar: '🏷️',
		text: {
			en: {
				name: 'Price Checker',
				title: 'Check your competitors’ prices every week, in one list.',
				metaTitle: 'Track competitor prices in a weekly list · Laamo',
				description:
					'A helper that opens the product pages you name every week, reads the current price on each and saves it to a list, so you see who changed what without visiting ten shops.',
				problem:
					'You want to know what the shop down the road and the three online sellers charge for the same things you sell. Checking ten pages every week is exactly the kind of task that gets dropped.',
				how: 'You put the pages to watch into a list once: one row per product page. Every Monday the helper opens each page, reads the price as it is shown and saves it with the date to your Prices list. Download the list as a spreadsheet to compare weeks.',
				stack: [
					{ group: 'when', text: 'Every Monday at 6:00' },
					{ group: 'look', text: 'Read my Price pages list' },
					{ group: 'look', text: 'Read each page' },
					{ group: 'think', text: 'Pull out the price' },
					{ group: 'do', text: 'Save to my Prices list' }
				],
				result: {
					title: 'Read 12 prices',
					lines: [
						'Oat milk 1 l at Bio-Markt Nord: 1,89 €',
						'Oat milk 1 l at naturkost-versand: 1,79 €',
						'10 more rows in your Prices list'
					]
				},
				questions: ['Which pages should I read? (your list of links)', 'How often?'],
				notes: [
					'The helper reads prices that are on the page when it opens. Prices that only show after a login, or after choosing a size, stay hidden from it.',
					'A helper that runs only when a page changes is coming soon.'
				],
				faq: [
					{
						q: 'Does it tell me when a price changes?',
						a: 'Today it saves every week’s prices to a list, where a change is easy to see. An alert only when something changed is coming soon.'
					},
					{
						q: 'Can it watch my suppliers’ prices as well?',
						a: 'Yes. Any public page with a price on it works: competitors, suppliers, or your own shop on a marketplace.'
					}
				]
			},
			de: {
				name: 'Preis-Check',
				title: 'Die Preise deiner Mitbewerber jede Woche prüfen, in einer Liste.',
				metaTitle: 'Preise der Konkurrenz wöchentlich beobachten · Laamo',
				description:
					'Ein Helfer, der jede Woche die Produktseiten öffnet, die du nennst, den aktuellen Preis liest und in einer Liste speichert. So siehst du, wer was geändert hat, ohne zehn Shops zu besuchen.',
				problem:
					'Du willst wissen, was der Laden um die Ecke und die drei Online-Händler für dieselben Sachen verlangen wie du. Jede Woche zehn Seiten durchzusehen ist genau die Aufgabe, die liegen bleibt.',
				how: 'Du trägst die Seiten einmal in eine Liste ein: eine Zeile pro Produktseite. Jeden Montag öffnet der Helfer jede Seite, liest den Preis, wie er dort steht, und speichert ihn mit Datum in deiner Preise-Liste. Lade die Liste als Tabelle herunter, um Wochen zu vergleichen.',
				stack: [
					{ group: 'when', text: 'Jeden Montag um 6:00' },
					{ group: 'look', text: 'Meine Liste „Preisseiten“ lesen' },
					{ group: 'look', text: 'Jede Seite lesen' },
					{ group: 'think', text: 'Den Preis herausziehen' },
					{ group: 'do', text: 'In meiner Preise-Liste speichern' }
				],
				result: {
					title: '12 Preise gelesen',
					lines: [
						'Haferdrink 1 l bei Bio-Markt Nord: 1,89 €',
						'Haferdrink 1 l bei naturkost-versand: 1,79 €',
						'10 weitere Zeilen in deiner Preise-Liste'
					]
				},
				questions: ['Welche Seiten soll ich lesen? (deine Liste mit Links)', 'Wie oft?'],
				notes: [
					'Der Helfer liest Preise, die beim Öffnen auf der Seite stehen. Preise, die erst nach einer Anmeldung oder nach Wahl einer Größe erscheinen, bleiben ihm verborgen.',
					'Ein Helfer, der nur läuft, wenn sich eine Seite ändert, kommt bald.'
				],
				faq: [
					{
						q: 'Sagt er mir Bescheid, wenn sich ein Preis ändert?',
						a: 'Heute speichert er die Preise jeder Woche in einer Liste, in der eine Änderung leicht zu sehen ist. Eine Nachricht nur bei Änderungen kommt bald.'
					},
					{
						q: 'Kann er auch die Preise meiner Lieferanten beobachten?',
						a: 'Ja. Jede öffentliche Seite mit einem Preis funktioniert: Mitbewerber, Lieferanten oder dein eigener Shop auf einem Marktplatz.'
					}
				]
			}
		}
	},
	{
		slug: 'email-reply-drafts',
		avatar: '✉️',
		text: {
			en: {
				name: 'Reply Drafter',
				title: 'Forward a customer email, get a reply draft in your tone.',
				metaTitle: 'Draft replies to customer emails in your tone · Laamo',
				description:
					'Forward a customer’s email to your helper and it writes a reply in your tone, using what it knows about your business. You check it, change what you like and send it from your own mailbox.',
				problem:
					'Most customer emails ask one of ten questions: opening hours, prices, a free slot, where the order is. Writing a friendly answer each time still takes the best part of an hour a day.',
				how: 'The helper has its own email address. Forward a customer email to it and it drafts a reply: polite, short, in your tone, with the facts about your business that you gave it. The draft waits in your Feed. Approve it, edit it or skip it. Laamo never answers your customers by itself.',
				stack: [
					{ group: 'when', text: 'When I forward an email' },
					{ group: 'think', text: 'Write a short, friendly reply' },
					{ group: 'ask', text: 'Check first' }
				],
				result: {
					title: 'Reply to Ms Keller is ready',
					lines: [
						'“Dear Ms Keller, thank you for asking. Yes, we can do the birthday cake for Saturday. For twelve people we suggest…”',
						'Approve · Edit · Skip'
					]
				},
				questions: [
					'What does your business offer? (asked once, remembered for all helpers)',
					'How should replies sound? Formal or first names?'
				],
				notes: [
					'The helper reads only what you forward to its address. It has no access to your mailbox and sends nothing to your customers.',
					'Only you can start it: by default it accepts emails from your own address alone.'
				],
				faq: [
					{
						q: 'Does Laamo send the reply for me?',
						a: 'No. The helper writes the draft; you send the reply from your own mailbox, so your customer hears from you and nothing leaves without your OK.'
					},
					{
						q: 'Can I forward emails automatically?',
						a: 'Yes. Every helper with an email address also has a private forwarding address for a rule in your mail program.'
					}
				]
			},
			de: {
				name: 'Antwort-Entwerfer',
				title: 'Kundenmail weiterleiten, Antwortentwurf in deinem Ton bekommen.',
				metaTitle: 'Antworten auf Kundenmails in deinem Ton entwerfen · Laamo',
				description:
					'Leite die E-Mail eines Kunden an deinen Helfer weiter, und er schreibt eine Antwort in deinem Ton, mit dem, was er über dein Geschäft weiß. Du prüfst sie, änderst, was du willst, und schickst sie aus deinem eigenen Postfach.',
				problem:
					'Die meisten Kundenmails stellen eine von zehn Fragen: Öffnungszeiten, Preise, ein freier Termin, wo die Bestellung bleibt. Jedes Mal freundlich zu antworten kostet trotzdem fast eine Stunde am Tag.',
				how: 'Der Helfer hat eine eigene E-Mail-Adresse. Leite eine Kundenmail dorthin weiter, und er entwirft eine Antwort: höflich, kurz, in deinem Ton, mit den Fakten über dein Geschäft, die du ihm gegeben hast. Der Entwurf wartet in deinem Feed. Gib ihn frei, ändere ihn oder überspring ihn. Laamo antwortet deinen Kunden nie von allein.',
				stack: [
					{ group: 'when', text: 'Wenn ich eine E-Mail weiterleite' },
					{ group: 'think', text: 'Eine kurze, freundliche Antwort schreiben' },
					{ group: 'ask', text: 'Erst prüfen' }
				],
				result: {
					title: 'Antwort an Frau Keller ist fertig',
					lines: [
						'„Liebe Frau Keller, danke für Ihre Anfrage. Ja, die Geburtstagstorte für Samstag schaffen wir. Für zwölf Personen empfehlen wir …“',
						'Freigeben · Ändern · Überspringen'
					]
				},
				questions: [
					'Was bietet dein Geschäft an? (einmal gefragt, für alle Helfer gemerkt)',
					'Wie sollen Antworten klingen? Per Sie oder per Du?'
				],
				notes: [
					'Der Helfer liest nur, was du an seine Adresse weiterleitest. Er hat keinen Zugriff auf dein Postfach und schickt nichts an deine Kunden.',
					'Nur du kannst ihn starten: Anfangs nimmt er nur E-Mails von deiner eigenen Adresse an.'
				],
				faq: [
					{
						q: 'Schickt Laamo die Antwort für mich ab?',
						a: 'Nein. Der Helfer schreibt den Entwurf; du schickst die Antwort aus deinem eigenen Postfach. So hört dein Kunde von dir, und nichts geht ohne dein OK raus.'
					},
					{
						q: 'Kann ich E-Mails automatisch weiterleiten?',
						a: 'Ja. Jeder Helfer mit E-Mail-Adresse hat auch eine private Weiterleitungsadresse für eine Regel in deinem Mailprogramm.'
					}
				]
			}
		}
	},
	{
		slug: 'trade-fair-finder',
		avatar: '🎪',
		text: {
			en: {
				name: 'Trade Fair Scout',
				title: 'Find trade fairs and networking events for your industry.',
				metaTitle: 'Find trade fairs and networking events for your industry · Laamo',
				description:
					'A helper that searches every week for trade fairs, markets and networking events in your industry and region. Swipe to keep the ones worth going to; they land in a list with date and place.',
				problem:
					'The fair where your next three customers were standing took place last month, an hour away. You found out from someone’s photos.',
				how: 'Every Monday the helper searches for newly announced trade fairs, markets, meetups and chamber events in your industry and within reach. It keeps the ones that fit a business like yours and shows them as cards. Swipe right to keep one: it goes into your Events list with its date, place and link.',
				stack: [
					{ group: 'when', text: 'Every Monday at 8:00' },
					{ group: 'look', text: 'Search trade fairs for florists in northern Germany' },
					{ group: 'think', text: 'Keep the ones worth a day’s trip' },
					{ group: 'think', text: 'Pull out date and place' },
					{ group: 'ask', text: 'Let me pick' },
					{ group: 'do', text: 'Save to my Events list' }
				],
				result: {
					title: '6 events to look at',
					lines: [
						'Regional flower and garden fair, Hamburg, 14–16 March',
						'Wedding suppliers’ evening, Lübeck, 2 April',
						'4 more to swipe through'
					]
				},
				questions: [
					'What does your business do? (asked once, remembered for all helpers)',
					'How far would you travel? A town, a region or a country.'
				],
				notes: [
					'Dates and places come from the events’ own pages. Check the organiser’s page before you book: events move.',
					'The helper learns from your swipes: what you skip, it shows less of next time.'
				],
				faq: [
					{
						q: 'Does it only find big trade fairs?',
						a: 'No. You describe what counts: fairs, local markets, meetups, chamber of commerce evenings or supplier days.'
					}
				]
			},
			de: {
				name: 'Messe-Scout',
				title: 'Messen und Netzwerk-Treffen für deine Branche finden.',
				metaTitle: 'Messen und Netzwerk-Veranstaltungen für deine Branche finden · Laamo',
				description:
					'Ein Helfer, der jede Woche nach Messen, Märkten und Netzwerk-Treffen in deiner Branche und Region sucht. Wisch dir die raus, die sich lohnen; sie landen mit Datum und Ort in einer Liste.',
				problem:
					'Die Messe, auf der deine nächsten drei Kunden standen, war letzten Monat, eine Stunde entfernt. Erfahren hast du es durch die Fotos von jemand anderem.',
				how: 'Jeden Montag sucht der Helfer nach neu angekündigten Messen, Märkten, Stammtischen und Kammer-Veranstaltungen in deiner Branche und in Reichweite. Er behält die, die zu einem Betrieb wie deinem passen, und zeigt sie als Karten. Wisch nach rechts, um eine zu behalten: Sie kommt mit Datum, Ort und Link in deine Termine-Liste.',
				stack: [
					{ group: 'when', text: 'Jeden Montag um 8:00' },
					{ group: 'look', text: 'Messen für Floristen in Norddeutschland suchen' },
					{ group: 'think', text: 'Die behalten, die eine Tagesreise wert sind' },
					{ group: 'think', text: 'Datum und Ort herausziehen' },
					{ group: 'ask', text: 'Lass mich wählen' },
					{ group: 'do', text: 'In meiner Termine-Liste speichern' }
				],
				result: {
					title: '6 Veranstaltungen zum Ansehen',
					lines: [
						'Regionale Blumen- und Gartenmesse, Hamburg, 14.–16. März',
						'Abend der Hochzeitsdienstleister, Lübeck, 2. April',
						'4 weitere zum Durchwischen'
					]
				},
				questions: [
					'Was macht dein Betrieb? (einmal gefragt, für alle Helfer gemerkt)',
					'Wie weit würdest du fahren? Ein Ort, eine Region oder ein Land.'
				],
				notes: [
					'Datum und Ort kommen von den Seiten der Veranstaltungen. Schau vor dem Buchen auf die Seite des Veranstalters: Termine verschieben sich.',
					'Der Helfer lernt aus deinem Wischen: Was du überspringst, zeigt er dir beim nächsten Mal seltener.'
				],
				faq: [
					{
						q: 'Findet er nur große Messen?',
						a: 'Nein. Du beschreibst, was zählt: Messen, lokale Märkte, Stammtische, Abende der Handelskammer oder Lieferantentage.'
					}
				]
			}
		}
	}
];
