-- A plan's revision goes up by one with every change to the plan, including ticked checklist
-- items. Saving an edited plan names the revision it started from, so an edit made on an old copy
-- is refused instead of silently undoing someone else's change.
ALTER TABLE issues ADD COLUMN plan_revision INTEGER NOT NULL DEFAULT 1 CHECK (plan_revision >= 1);
