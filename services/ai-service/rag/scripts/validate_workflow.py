#!/usr/bin/env python3
"""validate_workflow.py - structural checks on workflow/book-rag-system.json.

Run this after ANY edit to the workflow JSON. It catches the class of mistake that
n8n would otherwise only reveal at import or at run time: dangling connection
targets, missing required parameters, un-parameterised SQL tools, missing
credential blocks.

Usage:
    python scripts/validate_workflow.py [path-to-workflow.json]

Exit code 0 = no errors (warnings may still be printed), 1 = errors found.
"""

import json
import os
import sys

DEFAULT = os.path.join(
    os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
    "workflow",
    "book-rag-system.json",
)

# Types that must carry a credentials block (the user attaches the real credential).
NEEDS_CREDENTIALS = {
    "postgres",
    "postgresTool",
    "vectorStorePGVector",
    "memoryPostgresChat",
    "embeddingsGoogleGemini",
    "lmChatGoogleGemini",
    "webhook",
    "googleDrive",
    "googleDriveTrigger",
}

# Sub-nodes that must feed something via a non-main connection.
SUB_NODE_TYPES = {
    "embeddingsGoogleGemini",
    "lmChatGoogleGemini",
    "memoryPostgresChat",
    "documentDefaultDataLoader",
    "textSplitterRecursiveCharacterTextSplitter",
    "vectorStorePGVector",
}

VALID_CONNECTION_TYPES = {
    "main",
    "ai_tool",
    "ai_languageModel",
    "ai_memory",
    "ai_embedding",
    "ai_document",
    "ai_textSplitter",
    "ai_vectorStore",
    "ai_reranker",
    "ai_outputParser",
}


def validate(path):
    errors, warnings = [], []

    with open(path, encoding="utf-8") as fh:
        wf = json.load(fh)

    nodes = wf.get("nodes", [])
    names = [n["name"] for n in nodes]
    by_name = {n["name"]: n for n in nodes}

    if len(set(names)) != len(names):
        errors.append("duplicate node names")
    ids = [n.get("id") for n in nodes]
    if len(set(ids)) != len(ids):
        errors.append("duplicate node ids")

    # --- connections -------------------------------------------------------
    conns = wf.get("connections", {})
    for src, types in conns.items():
        if src not in by_name:
            errors.append(f"connection source '{src}' is not a node")
        for ctype, branches in types.items():
            if ctype not in VALID_CONNECTION_TYPES:
                warnings.append(f"{src}: unusual connection type '{ctype}'")
            for branch in branches:
                for c in branch:
                    if c["node"] not in by_name:
                        errors.append(f"{src} -> unknown node '{c['node']}'")

    # --- required parameters ----------------------------------------------
    for n in nodes:
        t = n["type"].split(".")[-1]
        p = n.get("parameters", {})
        blob = json.dumps(p)

        if t == "webhook":
            for k in ("httpMethod", "path", "responseMode"):
                if k not in p:
                    errors.append(f"{n['name']}: webhook missing '{k}'")
            if not n.get("webhookId"):
                warnings.append(f"{n['name']}: no webhookId (fine for static paths)")

        if t in ("postgresTool", "toolHttpRequest"):
            if not p.get("toolDescription"):
                warnings.append(f"{n['name']}: no toolDescription (the model sees this)")

        # The core safety rule: a SQL tool must never interpolate model-authored SQL.
        if t == "postgresTool":
            if "$fromAI" not in blob:
                errors.append(f"{n['name']}: SQL tool does not use $fromAI parameterisation")
            if "$fromAI('sql" in blob or "$fromAI('query" in blob:
                errors.append(f"{n['name']}: appears to accept a raw SQL string from the model")

        if t == "vectorStorePGVector":
            if not p.get("tableName"):
                errors.append(f"{n['name']}: missing tableName")
            cols = p.get("options", {}).get("columnNames", {}).get("values", {})
            expected = {
                "idColumnName": "id",
                "vectorColumnName": "embedding",
                "contentColumnName": "text",
                "metadataColumnName": "metadata",
            }
            for k, want in expected.items():
                if cols.get(k) not in (None, want):
                    warnings.append(
                        f"{n['name']}: {k}='{cols.get(k)}' differs from schema column '{want}'"
                    )

        if t == "agent":
            if not p.get("options", {}).get("systemMessage"):
                errors.append(f"{n['name']}: no systemMessage (agent would be ungrounded)")
            if p.get("promptType") == "define" and not p.get("text"):
                errors.append(f"{n['name']}: promptType define but no text")

        if t == "documentDefaultDataLoader":
            if p.get("textSplittingMode") == "simple":
                warnings.append(
                    f"{n['name']}: textSplittingMode='simple' means a connected "
                    "text splitter is silently ignored"
                )

        if t in NEEDS_CREDENTIALS and not n.get("credentials"):
            errors.append(f"{n['name']} ({t}) has no credentials block")

        # Metadata keys must not carry a leading '=' (the defect from the old template).
        for mv in p.get("options", {}).get("metadata", {}).get("metadataValues", []) or []:
            if str(mv.get("name", "")).startswith("="):
                errors.append(f"{n['name']}: metadata key '{mv['name']}' starts with '='")

    # --- sub-nodes must be wired ------------------------------------------
    for n in nodes:
        t = n["type"].split(".")[-1]
        if t in SUB_NODE_TYPES and n["name"] not in conns:
            errors.append(f"sub-node '{n['name']}' ({t}) has no outgoing connection")

    # --- exactly one embedding provider per vector table ------------------
    embedders = [n["name"] for n in nodes if n["type"].endswith("embeddingsGoogleGemini")]
    if len(embedders) > 1:
        warnings.append(
            f"{len(embedders)} Gemini embedding nodes present ({', '.join(embedders)}); "
            "they MUST be configured identically or retrieval silently breaks"
        )

    return wf, errors, warnings


def main():
    path = sys.argv[1] if len(sys.argv) > 1 else DEFAULT
    print(f"Validating: {path}")
    if not os.path.exists(path):
        print("FAIL: file not found")
        return 1

    try:
        wf, errors, warnings = validate(path)
    except json.JSONDecodeError as exc:
        print(f"FAIL: not valid JSON: {exc}")
        return 1

    trig = [n["name"] for n in wf["nodes"] if "Trigger" in n["type"] or n["type"].endswith("webhook")]
    tools = [n["name"] for n in wf["nodes"] if n["type"].endswith("Tool")]
    agents = [n["name"] for n in wf["nodes"] if n["type"].endswith("agent")]

    print(f"  nodes        : {len(wf['nodes'])}")
    print(f"  triggers     : {', '.join(trig) or 'none'}")
    print(f"  agents       : {', '.join(agents) or 'none'}")
    print(f"  agent tools  : {', '.join(tools) or 'none'}")
    print(f"  pinData empty: {not wf.get('pinData')}")

    for w in warnings:
        print(f"  WARN  {w}")
    for e in errors:
        print(f"  ERROR {e}")

    if errors:
        print(f"\nFAIL: {len(errors)} error(s), {len(warnings)} warning(s)")
        return 1
    print(f"\nPASS: 0 errors, {len(warnings)} warning(s)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
