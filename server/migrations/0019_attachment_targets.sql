-- An attachment can belong to the issue itself, to one of its subissues, or to one of its
-- findings. It always keeps its issue, so project scoping and issue lookups stay the same.
ALTER TABLE attachments ADD COLUMN subissue_id TEXT REFERENCES subissues (id) ON DELETE CASCADE;
ALTER TABLE attachments ADD COLUMN finding_id TEXT REFERENCES findings (id) ON DELETE CASCADE;

CREATE INDEX attachments_subissue ON attachments (subissue_id) WHERE subissue_id IS NOT NULL;
CREATE INDEX attachments_finding ON attachments (finding_id) WHERE finding_id IS NOT NULL;

CREATE TRIGGER attachments_one_target
BEFORE INSERT ON attachments
WHEN NEW.subissue_id IS NOT NULL AND NEW.finding_id IS NOT NULL
BEGIN
  SELECT RAISE(ABORT, 'an attachment belongs to a subissue or a finding, not both');
END;

CREATE TRIGGER attachments_subissue_same_issue
BEFORE INSERT ON attachments
WHEN NEW.subissue_id IS NOT NULL
  AND (SELECT issue_id FROM subissues WHERE id = NEW.subissue_id) IS NOT NEW.issue_id
BEGIN
  SELECT RAISE(ABORT, 'an attachment and its subissue must be on the same issue');
END;

CREATE TRIGGER attachments_finding_same_issue
BEFORE INSERT ON attachments
WHEN NEW.finding_id IS NOT NULL
  AND (SELECT r.issue_id FROM findings f JOIN review_rounds r ON r.id = f.review_round_id
       WHERE f.id = NEW.finding_id) IS NOT NEW.issue_id
BEGIN
  SELECT RAISE(ABORT, 'an attachment and its finding must be on the same issue');
END;
