-- Waiting list: "Something else" on the form comes with a few words of what
-- the person would like a helper to do.
ALTER TABLE waitlist ADD COLUMN wish text CHECK (length(wish) <= 300);
