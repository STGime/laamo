-- empty_list: deletes all rows of a list (Lists → Empty list), keeping the
-- list itself and the helpers that save to or read from it. What helpers
-- had read from it (list_reads) goes with the rows. Returns the number of
-- rows deleted.
CREATE FUNCTION empty_list(p_list uuid)
RETURNS jsonb
LANGUAGE plpgsql AS $$
DECLARE
  gone int;
BEGIN
  IF NOT public.is_service_role() THEN
    RAISE EXCEPTION 'empty_list is service only' USING ERRCODE = 'insufficient_privilege';
  END IF;
  DELETE FROM list_rows WHERE list_id = p_list;
  GET DIAGNOSTICS gone = ROW_COUNT;
  RETURN to_jsonb(gone);
END;
$$;
