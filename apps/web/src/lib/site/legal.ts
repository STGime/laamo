// Legal pages of the public site: imprint, privacy policy, terms. Operator:
// Eurobase OÜ (details as in the Eurobase console's legal.ts). Unlike the
// marketing copy, these name the services we use: the GDPR requires it.
//
// Keep in step with the code: the privacy policy lists what Laamo actually
// collects and which services get what (backend/internal/config, bricks).
import type { Lang } from '#lib/i18n/index.svelte.ts';

export const COMPANY = {
	name: 'Eurobase OÜ',
	street: 'Ahtri 12',
	city: '15551 Tallinn',
	registry: '17557586',
	email: 'contact@eurobase.app',
	privacyEmail: 'dpo@eurobase.app'
};

export const UPDATED = '2026-11-17';

/** A paragraph, or a list of points. */
export type Block = string | { list: string[] };

export interface LegalDoc {
	title: string;
	description: string;
	updated: string;
	sections: { h?: string; blocks: Block[] }[];
}

export type LegalSlug = 'imprint' | 'privacy' | 'terms';

const c = COMPANY;

const en: Record<LegalSlug, LegalDoc> = {
	imprint: {
		title: 'Imprint',
		description: 'Who runs Laamo: Eurobase OÜ, Tallinn, Estonia.',
		updated: 'Last updated: 17 November 2026',
		sections: [
			{
				blocks: [
					'Laamo is a service of',
					{
						list: [
							c.name,
							`${c.street}, ${c.city}, Estonia`,
							`Registry code ${c.registry} (Estonian Commercial Register)`,
							'Not registered for VAT',
							`Email: ${c.email}`,
							`Data protection: ${c.privacyEmail}`
						]
					}
				]
			}
		]
	},
	privacy: {
		title: 'Privacy policy',
		description:
			'What Laamo collects, why, how long we keep it, which services we use and your rights.',
		updated: 'Last updated: 17 November 2026',
		sections: [
			{
				h: '1. Who is responsible',
				blocks: [
					`Laamo is run by ${c.name}, ${c.street}, ${c.city}, Estonia, registry code ${c.registry} ("we"). We are the controller for the personal data described here. For questions about your data, write to ${c.privacyEmail}.`
				]
			},
			{
				h: '2. Visiting this website',
				blocks: [
					'When you open a page, our servers receive your IP address, your browser type, the page you asked for and the time. We use this to deliver the page and to protect the service against misuse (GDPR Art. 6(1)(f), legitimate interest). These server logs are kept for at most 30 days.',
					'The website sets no cookies and uses no analytics or advertising trackers. If you arrive through a share link, the share code is kept in your browser for the rest of your visit (session storage) so it can go along with your sign-up.'
				]
			},
			{
				h: '3. The waiting list',
				blocks: [
					'When you join the waiting list we store:',
					{
						list: [
							'your email address and language',
							'what you would like a helper to do, if you tell us (the options you pick and, for "Something else", your words)',
							'the page you signed up on and, if you came through someone’s share link, their share code',
							'your own share code, the version of the text you agreed to, and when you signed up, confirmed and were invited'
						]
					},
					'We use this to confirm your address, to invite you to Laamo, and to write to you now and then about what’s new in Laamo (GDPR Art. 6(1)(a), consent). You can withdraw your consent and leave the list at any time by writing to us; this does not affect what we did before.',
					'Your address only counts once you have used the confirmation link we email you. Addresses that are not confirmed within 7 days are deleted automatically. Confirmed entries are kept until you leave the list, and at the latest 24 months after you joined.',
					'To stop misuse of the form, your browser solves a small computing task before signing up, and we count sign-ups per IP address for 10 minutes. These counts are kept in memory only and are not stored.'
				]
			},
			{
				h: '4. Using Laamo',
				blocks: [
					'Once you have an account, we process what you put into Laamo and what your helpers make, so that the service works (GDPR Art. 6(1)(b), contract):',
					{
						list: [
							'your email address and sign-in data',
							'your workspace profile (for example your business, city, offer and signature)',
							'your helpers, their runs, results, Feed cards and lists',
							'emails you forward to a helper’s address, including attachments, and pictures your helpers draw',
							'connections to your chats (stored encrypted)',
							'usage records (runs, searches, AI costs) to keep within your plan'
						]
					},
					'We keep this data until you delete it or your account. We do not use your data to train AI models.'
				]
			},
			{
				h: '5. Services we use',
				blocks: [
					'We work with these providers. Each gets only what it needs for its job:',
					{
						list: [
							'Eurobase, our own platform, hosted by Scaleway (France): stores all Laamo data and runs the service.',
							'Cloudflare (USA): manages our domain and receives emails sent to helper addresses, which it passes on to us.',
							'Resend (USA): sends our emails, such as the waiting-list confirmation and invites.',
							'fal.ai and OpenRouter (USA), running a model by Mistral AI (France): the AI behind Think bricks and pictures. They receive the text a brick works on.',
							'Brave Search (USA): receives the search terms of Search the web.',
							'OpenStreetMap Foundation (United Kingdom): receives place names and the kind of business when a helper finds businesses.',
							'Geoapify (Germany): draws small maps on business cards from their coordinates.',
							'Chat services you connect: receive the messages your helpers post there.',
							'Websites your helpers read are fetched by our servers; those websites see our servers, not you.'
						]
					},
					'We may also share data with professional advisers under confidentiality, and with authorities where the law requires it.'
				]
			},
			{
				h: '6. Transfers outside the EU',
				blocks: [
					'Laamo data is stored in the EU (France). Some providers above are in the USA or the United Kingdom. For the UK, the European Commission has decided that data protection there is adequate. For providers in the USA, transfers rely on the EU standard contractual clauses and, where the provider is certified, the EU-US Data Privacy Framework.'
				]
			},
			{
				h: '7. Your rights',
				blocks: [
					'Under the GDPR you can ask us for a copy of your data, have it corrected or deleted, restrict or object to its use, get it in a machine-readable format, and withdraw consent at any time. Write to ' +
						c.privacyEmail +
						'. We answer within one month and do not charge for it.',
					'You can also complain to a data protection authority. Ours is the Estonian Data Protection Inspectorate (Andmekaitse Inspektsioon), Tatari 39, 10134 Tallinn, www.aki.ee. You may also turn to the authority in the EU country where you live.',
					'We do not make automated decisions about you that have legal or similarly significant effects.'
				]
			},
			{
				h: '8. Security',
				blocks: [
					'All traffic is encrypted (TLS). Each workspace’s data is separated in the database, chat connections are stored encrypted, and confirmation links are stored only in scrambled (hashed) form. If a data breach is likely to put you at high risk, we tell you without undue delay.'
				]
			},
			{
				h: '9. Children',
				blocks: [
					`Laamo is not meant for children under 16. If you think a child has given us personal data, write to ${c.privacyEmail} and we will delete it.`
				]
			},
			{
				h: '10. Changes',
				blocks: [
					'If we change this policy in an important way, we will post the new version here and tell you by email before it takes effect.'
				]
			}
		]
	},
	terms: {
		title: 'Terms of use',
		description: 'The rules for using Laamo, in plain words.',
		updated: 'Last updated: 17 November 2026',
		sections: [
			{
				h: '1. Who we are',
				blocks: [
					`Laamo is a service of ${c.name}, ${c.street}, ${c.city}, Estonia, registry code ${c.registry}. These terms apply when you use Laamo. Our privacy policy explains how we handle your data.`
				]
			},
			{
				h: '2. Early access',
				blocks: [
					'Laamo is new. During early access we let people in a few at a time, features may change, and the service may sometimes be unavailable. Early access is free; we will tell you the prices before you pay for anything.'
				]
			},
			{
				h: '3. Your account',
				blocks: [
					'You need to be at least 16. Use an email address you can read, keep access to it safe, and tell us if you think someone else used your account.'
				]
			},
			{
				h: '4. What your helpers do',
				blocks: [
					'You decide what your helpers do, and you are responsible for it. Anything that leaves Laamo waits for your OK by default; if you remove that step, the helper acts without asking.',
					'Emails to new contacts are drafts that you send yourself, from your own mailbox. Rules for marketing emails differ by country (in some, you need the recipient’s consent first). Following them is your responsibility.'
				]
			},
			{
				h: '5. What is not allowed',
				blocks: [
					{
						list: [
							'sending spam or unsolicited bulk messages, or using bought or scraped address lists',
							'anything illegal, misleading or harmful, or that breaks other people’s rights',
							'pretending to be someone else',
							'trying to break, overload or get around the limits of Laamo or of websites your helpers read',
							'using Laamo to build a competing service from its output'
						]
					},
					'If you break these rules we may pause your helpers or close your account. Where we can, we tell you first.'
				]
			},
			{
				h: '6. AI results',
				blocks: [
					'Helpers use AI to read, sort, sum up, write and draw. Results can be wrong or incomplete. Check them before you rely on them or send them to anyone.'
				]
			},
			{
				h: '7. Your content',
				blocks: [
					'What you put into Laamo and what your helpers make for you stays yours. You allow us to store and process it only to run Laamo for you. Services you connect, such as your chats, have their own terms.'
				]
			},
			{
				h: '8. Ending',
				blocks: [
					'You can stop using Laamo and delete your account at any time. We may end the service with 30 days’ notice, and you can download your lists before that.'
				]
			},
			{
				h: '9. Liability',
				blocks: [
					'We are fully liable for damage we cause intentionally or through gross negligence, and for injury to life, body or health. Otherwise we are liable only for breaches of essential duties and only for damage that was typical and foreseeable. During free early access our liability is limited as far as the law allows. Your rights as a consumer under mandatory law are not affected.'
				]
			},
			{
				h: '10. Changes to these terms',
				blocks: [
					'If we change these terms, we email you at least 30 days before the change takes effect. If you don’t agree, you can delete your account before then.'
				]
			},
			{
				h: '11. Law',
				blocks: [
					'Estonian law applies. If you are a consumer, you keep the protection of the mandatory law of the country where you live, and you can go to court there.',
					`Questions: ${c.email}`
				]
			}
		]
	}
};

const de: Record<LegalSlug, LegalDoc> = {
	imprint: {
		title: 'Impressum',
		description: 'Wer Laamo betreibt: Eurobase OÜ, Tallinn, Estland.',
		updated: 'Stand: 17. November 2026',
		sections: [
			{
				blocks: [
					'Laamo ist ein Dienst der',
					{
						list: [
							c.name,
							`${c.street}, ${c.city}, Estland`,
							`Registernummer ${c.registry} (Estnisches Handelsregister)`,
							'Nicht umsatzsteuerlich registriert',
							`E-Mail: ${c.email}`,
							`Datenschutz: ${c.privacyEmail}`
						]
					}
				]
			}
		]
	},
	privacy: {
		title: 'Datenschutzerklärung',
		description:
			'Was Laamo erhebt, wozu, wie lange wir es speichern, welche Dienste wir nutzen und welche Rechte du hast.',
		updated: 'Stand: 17. November 2026',
		sections: [
			{
				h: '1. Wer verantwortlich ist',
				blocks: [
					`Laamo wird von der ${c.name}, ${c.street}, ${c.city}, Estland, Registernummer ${c.registry} („wir“) betrieben. Wir sind Verantwortliche für die hier beschriebenen personenbezogenen Daten. Fragen zu deinen Daten richtest du an ${c.privacyEmail}.`
				]
			},
			{
				h: '2. Besuch dieser Website',
				blocks: [
					'Wenn du eine Seite öffnest, erhalten unsere Server deine IP-Adresse, deinen Browsertyp, die aufgerufene Seite und die Uhrzeit. Wir nutzen das, um die Seite auszuliefern und den Dienst vor Missbrauch zu schützen (Art. 6 Abs. 1 lit. f DSGVO, berechtigtes Interesse). Diese Server-Logs werden höchstens 30 Tage aufbewahrt.',
					'Die Website setzt keine Cookies und nutzt keine Analyse- oder Werbe-Tracker. Kommst du über einen Teilen-Link, wird der Code für den Rest deines Besuchs in deinem Browser gespeichert (Session Storage), damit er mit deiner Anmeldung mitgeschickt werden kann.'
				]
			},
			{
				h: '3. Die Warteliste',
				blocks: [
					'Wenn du dich in die Warteliste einträgst, speichern wir:',
					{
						list: [
							'deine E-Mail-Adresse und Sprache',
							'was ein Helfer für dich tun soll, wenn du es angibst (die gewählten Optionen und, bei „Etwas anderes“, deine Worte)',
							'die Seite, auf der du dich eingetragen hast, und, wenn du über einen Teilen-Link kamst, dessen Code',
							'deinen eigenen Teilen-Code, die Version des Textes, dem du zugestimmt hast, und wann du dich eingetragen, bestätigt und eine Einladung bekommen hast'
						]
					},
					'Wir nutzen das, um deine Adresse zu bestätigen, dich zu Laamo einzuladen und dir ab und zu zu schreiben, was es Neues bei Laamo gibt (Art. 6 Abs. 1 lit. a DSGVO, Einwilligung). Du kannst deine Einwilligung jederzeit widerrufen und die Liste verlassen, indem du uns schreibst; das berührt nicht, was wir vorher getan haben.',
					'Deine Adresse zählt erst, wenn du den Bestätigungslink aus unserer E-Mail benutzt hast. Adressen, die nicht innerhalb von 7 Tagen bestätigt werden, löschen wir automatisch. Bestätigte Einträge bewahren wir auf, bis du die Liste verlässt, höchstens aber 24 Monate nach deiner Anmeldung.',
					'Gegen Missbrauch des Formulars löst dein Browser vor der Anmeldung eine kleine Rechenaufgabe, und wir zählen Anmeldungen pro IP-Adresse für 10 Minuten. Diese Zählung liegt nur im Arbeitsspeicher und wird nicht gespeichert.'
				]
			},
			{
				h: '4. Laamo nutzen',
				blocks: [
					'Sobald du ein Konto hast, verarbeiten wir, was du in Laamo eingibst und was deine Helfer erstellen, damit der Dienst funktioniert (Art. 6 Abs. 1 lit. b DSGVO, Vertrag):',
					{
						list: [
							'deine E-Mail-Adresse und Anmeldedaten',
							'dein Arbeitsbereich-Profil (zum Beispiel dein Geschäft, deine Stadt, dein Angebot und deine Signatur)',
							'deine Helfer, ihre Läufe, Ergebnisse, Feed-Karten und Listen',
							'E-Mails, die du an die Adresse eines Helfers weiterleitest, samt Anhängen, und Bilder, die deine Helfer zeichnen',
							'Verbindungen zu deinen Chats (verschlüsselt gespeichert)',
							'Nutzungsdaten (Läufe, Suchen, KI-Kosten), um im Rahmen deines Plans zu bleiben'
						]
					},
					'Wir bewahren diese Daten auf, bis du sie oder dein Konto löschst. Wir nutzen deine Daten nicht, um KI-Modelle zu trainieren.'
				]
			},
			{
				h: '5. Dienste, die wir nutzen',
				blocks: [
					'Wir arbeiten mit diesen Anbietern. Jeder bekommt nur, was er für seine Aufgabe braucht:',
					{
						list: [
							'Eurobase, unsere eigene Plattform, gehostet von Scaleway (Frankreich): speichert alle Laamo-Daten und betreibt den Dienst.',
							'Cloudflare (USA): verwaltet unsere Domain und empfängt E-Mails an Helfer-Adressen, die es an uns weitergibt.',
							'Resend (USA): verschickt unsere E-Mails, etwa die Bestätigung für die Warteliste und Einladungen.',
							'fal.ai und OpenRouter (USA) mit einem Modell von Mistral AI (Frankreich): die KI hinter Denken-Bausteinen und Bildern. Sie erhalten den Text, mit dem ein Baustein arbeitet.',
							'Brave Search (USA): erhält die Suchbegriffe von „Im Web suchen“.',
							'OpenStreetMap Foundation (Vereinigtes Königreich): erhält Ortsnamen und die Art des Betriebs, wenn ein Helfer Betriebe sucht.',
							'Geoapify (Deutschland): zeichnet aus deren Koordinaten kleine Karten auf Betriebs-Karten.',
							'Chat-Dienste, die du verbindest: erhalten die Nachrichten, die deine Helfer dort posten.',
							'Websites, die deine Helfer lesen, werden von unseren Servern abgerufen; diese Websites sehen unsere Server, nicht dich.'
						]
					},
					'Außerdem können wir Daten unter Verschwiegenheit an Berater weitergeben und an Behörden, wenn das Gesetz es verlangt.'
				]
			},
			{
				h: '6. Übermittlung außerhalb der EU',
				blocks: [
					'Laamo-Daten werden in der EU (Frankreich) gespeichert. Einige der Anbieter oben sitzen in den USA oder im Vereinigten Königreich. Für das Vereinigte Königreich hat die EU-Kommission ein angemessenes Datenschutzniveau festgestellt. Bei Anbietern in den USA stützt sich die Übermittlung auf die EU-Standardvertragsklauseln und, wenn der Anbieter zertifiziert ist, auf das EU-US Data Privacy Framework.'
				]
			},
			{
				h: '7. Deine Rechte',
				blocks: [
					'Nach der DSGVO kannst du eine Kopie deiner Daten verlangen, sie berichtigen oder löschen lassen, ihre Nutzung einschränken oder ihr widersprechen, sie in einem maschinenlesbaren Format erhalten und Einwilligungen jederzeit widerrufen. Schreib an ' +
						c.privacyEmail +
						'. Wir antworten innerhalb eines Monats und verlangen dafür nichts.',
					'Du kannst dich auch bei einer Datenschutzbehörde beschweren. Unsere ist die estnische Datenschutzbehörde (Andmekaitse Inspektsioon), Tatari 39, 10134 Tallinn, www.aki.ee. Du kannst dich auch an die Behörde in dem EU-Land wenden, in dem du lebst.',
					'Wir treffen keine automatisierten Entscheidungen über dich, die rechtliche oder ähnlich erhebliche Wirkung haben.'
				]
			},
			{
				h: '8. Sicherheit',
				blocks: [
					'Alle Verbindungen sind verschlüsselt (TLS). Die Daten jedes Arbeitsbereichs sind in der Datenbank getrennt, Chat-Verbindungen werden verschlüsselt gespeichert und Bestätigungslinks nur in unkenntlich gemachter (gehashter) Form. Wenn eine Datenpanne dich wahrscheinlich einem hohen Risiko aussetzt, informieren wir dich unverzüglich.'
				]
			},
			{
				h: '9. Kinder',
				blocks: [
					`Laamo ist nicht für Kinder unter 16 gedacht. Wenn du glaubst, dass ein Kind uns personenbezogene Daten gegeben hat, schreib an ${c.privacyEmail}, und wir löschen sie.`
				]
			},
			{
				h: '10. Änderungen',
				blocks: [
					'Wenn wir diese Erklärung wesentlich ändern, veröffentlichen wir die neue Fassung hier und informieren dich per E-Mail, bevor sie gilt.'
				]
			}
		]
	},
	terms: {
		title: 'Nutzungsbedingungen',
		description: 'Die Regeln für die Nutzung von Laamo, in einfachen Worten.',
		updated: 'Stand: 17. November 2026',
		sections: [
			{
				h: '1. Wer wir sind',
				blocks: [
					`Laamo ist ein Dienst der ${c.name}, ${c.street}, ${c.city}, Estland, Registernummer ${c.registry}. Diese Bedingungen gelten, wenn du Laamo nutzt. Wie wir mit deinen Daten umgehen, steht in unserer Datenschutzerklärung.`
				]
			},
			{
				h: '2. Früher Zugang',
				blocks: [
					'Laamo ist neu. Während des frühen Zugangs lassen wir nach und nach Leute rein, Funktionen können sich ändern und der Dienst kann zeitweise nicht verfügbar sein. Der frühe Zugang ist kostenlos; die Preise nennen wir dir, bevor du für etwas bezahlst.'
				]
			},
			{
				h: '3. Dein Konto',
				blocks: [
					'Du musst mindestens 16 Jahre alt sein. Nutze eine E-Mail-Adresse, die du lesen kannst, schütze den Zugang dazu und sag uns Bescheid, wenn du glaubst, dass jemand anderes dein Konto genutzt hat.'
				]
			},
			{
				h: '4. Was deine Helfer tun',
				blocks: [
					'Du entscheidest, was deine Helfer tun, und bist dafür verantwortlich. Alles, was Laamo verlässt, wartet standardmäßig auf dein OK; entfernst du diesen Schritt, handelt der Helfer ohne zu fragen.',
					'E-Mails an neue Kontakte sind Entwürfe, die du selbst aus deinem eigenen Postfach schickst. Die Regeln für Werbe-E-Mails unterscheiden sich je nach Land (in manchen brauchst du vorher die Einwilligung des Empfängers). Sie einzuhalten liegt bei dir.'
				]
			},
			{
				h: '5. Was nicht erlaubt ist',
				blocks: [
					{
						list: [
							'Spam oder unerwünschte Massennachrichten verschicken oder gekaufte oder abgegriffene Adresslisten nutzen',
							'alles, was rechtswidrig, irreführend oder schädlich ist oder die Rechte anderer verletzt',
							'sich als jemand anderes ausgeben',
							'versuchen, Laamo oder die Websites, die deine Helfer lesen, zu stören, zu überlasten oder ihre Grenzen zu umgehen',
							'Laamo nutzen, um aus seinen Ergebnissen einen Konkurrenzdienst zu bauen'
						]
					},
					'Wenn du gegen diese Regeln verstößt, können wir deine Helfer pausieren oder dein Konto schließen. Wo es geht, sagen wir dir vorher Bescheid.'
				]
			},
			{
				h: '6. KI-Ergebnisse',
				blocks: [
					'Helfer nutzen KI, um zu lesen, zu sortieren, zusammenzufassen, zu schreiben und zu zeichnen. Ergebnisse können falsch oder unvollständig sein. Prüfe sie, bevor du dich darauf verlässt oder sie jemandem schickst.'
				]
			},
			{
				h: '7. Deine Inhalte',
				blocks: [
					'Was du in Laamo eingibst und was deine Helfer für dich erstellen, bleibt deins. Du erlaubst uns, es nur zu speichern und zu verarbeiten, um Laamo für dich zu betreiben. Dienste, die du verbindest, etwa deine Chats, haben eigene Bedingungen.'
				]
			},
			{
				h: '8. Ende',
				blocks: [
					'Du kannst Laamo jederzeit nicht mehr nutzen und dein Konto löschen. Wir können den Dienst mit 30 Tagen Vorlauf beenden; deine Listen kannst du vorher herunterladen.'
				]
			},
			{
				h: '9. Haftung',
				blocks: [
					'Für Schäden, die wir vorsätzlich oder grob fahrlässig verursachen, und für Verletzungen von Leben, Körper oder Gesundheit haften wir voll. Sonst haften wir nur für die Verletzung wesentlicher Pflichten und nur für typische, vorhersehbare Schäden. Während des kostenlosen frühen Zugangs ist unsere Haftung so weit beschränkt, wie das Gesetz es erlaubt. Deine Rechte als Verbraucher nach zwingendem Recht bleiben unberührt.'
				]
			},
			{
				h: '10. Änderungen dieser Bedingungen',
				blocks: [
					'Wenn wir diese Bedingungen ändern, schreiben wir dir mindestens 30 Tage, bevor die Änderung gilt. Bist du nicht einverstanden, kannst du dein Konto vorher löschen.'
				]
			},
			{
				h: '11. Recht',
				blocks: [
					'Es gilt estnisches Recht. Als Verbraucher behältst du den Schutz der zwingenden Vorschriften des Landes, in dem du lebst, und kannst dort vor Gericht gehen.',
					`Fragen: ${c.email}`
				]
			}
		]
	}
};

export const LEGAL: Record<Lang, Record<LegalSlug, LegalDoc>> = { en, de };
export const LEGAL_SLUGS: LegalSlug[] = ['imprint', 'privacy', 'terms'];
