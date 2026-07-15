#!/usr/bin/env python3
"""
CLIProxyAPI Safe Configuration Synchronizer
-------------------------------------------
Expert-level synchronization utility for distributed AI gateway deployments.
This script performs an AST-level (Abstract Syntax Tree) semantic merge between
the static upstream 'config.yaml' and the live operational 'pgstore/config/config.yaml'.

WHY THIS IS NECESSARY (ARCHITECTURAL CONTEXT):
Blindly overwriting the live configuration file (e.g., via `cp -f`) is a critical 
system vulnerability. It leads to:
1. State Erasure: Wipes out dynamically issued API keys, client billing records, and quotas.
2. Deployment Race Conditions: Writing during an active lock cycle corrupts the ledger.

This tool safely propagates structural updates (like new models, routing rules, or bug fixes)
while isolating and safeguarding the production state data.
"""

import sys
import os
import shutil
from pathlib import Path

try:
    from ruamel.yaml import YAML
except ImportError:
    print("ERROR: ruamel.yaml is required for semantic merging.")
    print("Please run: pip install ruamel.yaml")
    sys.exit(1)

# Configuration Paths
ROOT_DIR = Path(__file__).resolve().parent.parent
SOURCE_CONFIG = ROOT_DIR / "config.yaml"
LIVE_CONFIG = ROOT_DIR / "pgstore" / "config" / "config.yaml"

# The "Stateful" fields that must NEVER be overwritten by the static template.
# These represent the live operational database of the AI gateway.
STATEFUL_KEYS = [
    "api-keys",                     # Dynamic client API keys
    "api-key-models",               # Custom model mappings per key
    "api-key-limits",               # Rate limits and bounds per key
    "gemini-api-key",               # Configured upstream Gemini keys
    "codex-api-key",                # Configured upstream Codex keys
    "claude-api-key",               # Configured upstream Claude keys
    "openai-compatibility",         # Third-party OpenAI compatible provider definitions
    "vertex-api-key",               # Vertex AI keys
]

def main():
    if not SOURCE_CONFIG.exists():
        print(f"FATAL: Upstream source config not found at {SOURCE_CONFIG}")
        sys.exit(1)

    if not LIVE_CONFIG.exists():
        print(f"INFO: Live config not found at {LIVE_CONFIG}. Initializing from source...")
        LIVE_CONFIG.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(SOURCE_CONFIG, LIVE_CONFIG)
        print("SUCCESS: Initialized live configuration safely.")
        sys.exit(0)

    print("Executing Semantic Config Merge...")
    yaml = YAML()
    yaml.preserve_quotes = True
    yaml.indent(mapping=2, sequence=4, offset=2)
    yaml.width = 4096  # Prevent arbitrary line wrapping

    # 1. Load Live State (Operational Truth)
    with open(LIVE_CONFIG, 'r', encoding='utf-8') as f:
        try:
            live_data = yaml.load(f)
        except Exception as e:
            print(f"FATAL: Failed to parse LIVE config. {e}")
            sys.exit(1)

    # 2. Load New Upstream Template (Structural Truth)
    with open(SOURCE_CONFIG, 'r', encoding='utf-8') as f:
        try:
            new_data = yaml.load(f)
        except Exception as e:
            print(f"FATAL: Failed to parse SOURCE config. {e}")
            sys.exit(1)

    # 3. Perform the Semantic Merge
    # We want the structure of the NEW config, but we inject the STATE from the LIVE config.
    merged_count = 0
    for key in STATEFUL_KEYS:
        if key in live_data:
            new_data[key] = live_data[key]
            merged_count += 1
            print(f"  [+] Merged stateful sector: '{key}'")
    
    # Special Handle Nested State: Post-Pay Billing Clients
    # We want to inherit new billing structural rules, but keep the client ledger.
    if 'post-pay-billing' in new_data and 'post-pay-billing' in live_data:
        live_clients = live_data['post-pay-billing'].get('clients', {})
        if live_clients:
            new_data['post-pay-billing']['clients'] = live_clients
            print("  [+] Merged stateful sector: 'post-pay-billing.clients'")
            merged_count += 1

    # Special Handle Nested State: AmpCode API Keys mappings
    if 'ampcode' in new_data and 'ampcode' in live_data:
        if 'upstream-api-keys' in live_data['ampcode']:
            new_data['ampcode']['upstream-api-keys'] = live_data['ampcode']['upstream-api-keys']
            print("  [+] Merged stateful sector: 'ampcode.upstream-api-keys'")
            merged_count += 1

    # 4. Safely write back to the live directory (using atomic swap)
    temp_target = LIVE_CONFIG.with_suffix('.yaml.tmp')
    try:
        with open(temp_target, 'w', encoding='utf-8') as f:
            yaml.dump(new_data, f)
        
        # Atomic replace
        temp_target.replace(LIVE_CONFIG)
        print(f"SUCCESS: Config synchronization completed. {merged_count} stateful sectors preserved.")
    except Exception as e:
        if temp_target.exists():
            temp_target.unlink()
        print(f"FATAL: Failed to write merged config to disk. System state untouched. {e}")
        sys.exit(1)

if __name__ == "__main__":
    main()
