CREATE TABLE IF NOT EXISTS project_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    inviter_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invitee_login text NOT NULL,
    role text NOT NULL CHECK (role IN ('admin', 'member', 'viewer')),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'declined')),
    expires_at timestamptz NOT NULL DEFAULT (now() + interval '7 days'),
    created_at timestamptz NOT NULL DEFAULT now(),
    responded_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS project_invitations_pending_idx
    ON project_invitations(project_id, lower(invitee_login))
    WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS project_invitations_login_idx
    ON project_invitations(lower(invitee_login), created_at DESC)
    WHERE status = 'pending';

ALTER TABLE project_invitations ENABLE ROW LEVEL SECURITY;
