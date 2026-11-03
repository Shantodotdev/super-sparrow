CREATE TABLE subissue_labels (
  subissue_id TEXT NOT NULL REFERENCES subissues (id) ON DELETE CASCADE,
  label_id TEXT NOT NULL REFERENCES labels (id) ON DELETE CASCADE,
  PRIMARY KEY (subissue_id, label_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX subissue_labels_label ON subissue_labels (label_id);

CREATE TRIGGER subissue_labels_same_project
BEFORE INSERT ON subissue_labels
WHEN (SELECT i.project_id FROM subissues s JOIN issues i ON i.id = s.issue_id WHERE s.id = NEW.subissue_id)
  IS NOT (SELECT project_id FROM labels WHERE id = NEW.label_id)
BEGIN
  SELECT RAISE(ABORT, 'a label must be in the same project as its subissue');
END;
