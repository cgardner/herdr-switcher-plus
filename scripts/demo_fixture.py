"""Build a throwaway session fixture for documentation screenshots.

The sessions are invented. They are chosen to show what the switcher is for:
a repository with several linked worktrees, a space whose name hides its
branch, an ordinary checkout in a differently named space, a space that is no
checkout at all, and ages spanning all three colour bands.
"""

import json
import os
import sys
import time
import uuid

FIXTURE = sys.argv[1]
HOME = os.path.join(FIXTURE, "home")
CHECKOUTS = os.path.join(FIXTURE, "checkouts")

# (space, repo, linked, branch, minutes ago, status, role, message)
SESSIONS = [
    ("api-gateway", "api-gateway", False, "main", 0.2, "working", "assistant",
     "Rate limiter is in. Added the burst case to the table test and it passes."),
    ("auth-service", "platform", True, "auth-service", 2, "blocked", "assistant",
     "Two ways to expire the refresh token. Which do you want?"),
    ("billing", "platform", True, "billing", 8, "done", "assistant",
     "Backfill finished. 41,882 invoices migrated, none rejected."),
    ("web-client", "web-client", False, "redesign-nav", 45, "idle", "user",
     "Make the sidebar collapse below 900px."),
    ("dotfiles", None, None, None, 190, "idle", "assistant",
     "Symlinks rebuilt. Your old zsh config is saved as zshrc.backup."),
    ("notifications", "platform", True, "notifications", 1500, "idle", "assistant",
     "Digest job now batches per hour. Left the per-event path untouched."),
    ("infra", "infra-terraform", False, "main", 2900, "idle", "user",
     "Plan looks right, apply it."),
    ("scratchpad", None, None, None, 13000, "idle", "assistant",
     "Done for now."),
]

now = time.time()
agents, workspaces = [], []

for i, (space, repo, linked, branch, mins, status, role, text) in enumerate(SESSIONS):
    ws_id = "w%d" % (i + 1)
    session_id = str(uuid.uuid4())
    checkout = None

    if repo:
        checkout = os.path.join(CHECKOUTS, space)
        git = os.path.join(checkout, ".git")
        os.makedirs(git, exist_ok=True)
        with open(os.path.join(git, "HEAD"), "w") as f:
            f.write("ref: refs/heads/%s\n" % branch)
        workspaces.append({
            "workspace_id": ws_id, "label": space, "number": i + 1,
            "worktree": {
                "repo_name": repo, "repo_root": checkout,
                "checkout_path": checkout, "is_linked_worktree": bool(linked),
            },
        })
    else:
        checkout = os.path.join(CHECKOUTS, space)
        os.makedirs(checkout, exist_ok=True)
        workspaces.append({"workspace_id": ws_id, "label": space, "number": i + 1})

    agents.append({
        "agent": "claude", "agent_status": status, "cwd": checkout,
        "pane_id": "%s:p1" % ws_id, "tab_id": "%s:t1" % ws_id,
        "workspace_id": ws_id, "terminal_title_stripped": space,
        "state_change_seq": 100 - i,
        "agent_session": {"kind": "id", "value": session_id},
    })

    # The transcript the age and the preview come from.
    project = os.path.join(HOME, ".claude", "projects", checkout.replace("/", "-"))
    os.makedirs(project, exist_ok=True)
    stamp = time.strftime("%Y-%m-%dT%H:%M:%S", time.gmtime(now - mins * 60)) + ".000Z"
    with open(os.path.join(project, session_id + ".jsonl"), "w") as f:
        f.write(json.dumps({
            "type": role, "timestamp": stamp,
            "message": {"role": role, "content": text},
        }) + "\n")
        # A bookkeeping record after the message, which is what makes the file
        # modification time useless and the transcript scan necessary.
        f.write(json.dumps({"type": "artifact-autoreact-ledger", "v": 1}) + "\n")

os.makedirs(os.path.join(FIXTURE, "config"), exist_ok=True)
os.makedirs(os.path.join(FIXTURE, "state"), exist_ok=True)
with open(os.path.join(FIXTURE, "snapshot.json"), "w") as f:
    json.dump({"id": "demo", "result": {"snapshot": {
        "agents": agents, "workspaces": workspaces, "tabs": [],
        "panes": agents, "focused_pane_id": "w1:p1",
    }}}, f)
