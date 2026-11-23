// The time zone a new workspace gets: the browser's, if it names a place.
//
// A workspace's zone has to be an IANA place ("Europe/Berlin") or "UTC":
// the API and the database refuse abbreviations and offsets ("CET",
// "UTC+3"), which don't follow a place's summer time. Nearly every browser
// reports a place. The few that don't (a computer set to "CET", or no zone
// at all) start in Europe/Berlin, so signing up never fails on it; the zone
// can be changed in the workspace's settings.

const PLACE = /^(UTC|[A-Za-z_]+(\/[A-Za-z0-9_+-]+)+)$/;

export const DEFAULT_TIMEZONE = 'Europe/Berlin';

export function workspaceTimezone(
	tz: string | undefined = Intl.DateTimeFormat().resolvedOptions().timeZone
): string {
	return tz && PLACE.test(tz) ? tz : DEFAULT_TIMEZONE;
}
