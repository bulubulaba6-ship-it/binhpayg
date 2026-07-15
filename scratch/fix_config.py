import sys
import shutil

# We use config.render.yaml as the clean source of truth
with open('c:/gitroot/backup/config.render.yaml', 'r', encoding='utf-8') as f:
    text = f.read()

# 1. Add to api-keys
text = text.replace(
    '  - fink_pro_e2fc714c4727ee9395f324cd2e7f331f',
    '  - fink_pro_e2fc714c4727ee9395f324cd2e7f331f\n  - fink_max_3d6be4e0c6f57d845c49b853b246053a'
)

# 2. Add to api-key-models
models_block = """  "fink_max_3d6be4e0c6f57d845c49b853b246053a":
    claude:
      - claude-opus-4-8
      - claude-opus-4-7
      - claude-opus-4-6
      - claude-sonnet-4-6
      - claude-haiku-4-5
    openai:
      - gpt-5.5
      - gpt-5.4
      - gpt-5.4-mini
      - gpt-5.3-codex
      - gpt-5.3-codex-spark
      - gpt-5.2"""

text = text.replace(
    '      - claude-haiku-4-5\n\n# ==============================================================================\n# GENERAL SETTINGS',
    '      - claude-haiku-4-5\n' + models_block + '\n\n# ==============================================================================\n# GENERAL SETTINGS'
)

# 3. Add to api-key-limits
text = text.replace(
    '  "fink_pro_e2fc714c4727ee9395f324cd2e7f331f": 2000    # Pro:    2,000 cr/5h',
    '  "fink_pro_e2fc714c4727ee9395f324cd2e7f331f": 2000    # Pro:    2,000 cr/5h\n  "fink_max_3d6be4e0c6f57d845c49b853b246053a": 10000   # Max:    10,000 cr/5h'
)

# 4. Add to post-pay-billing.clients
text = text.replace(
    '    "fink_pro_e2fc714c4727ee9395f324cd2e7f331f":\n      credit-limit: 2000',
    '    "fink_pro_e2fc714c4727ee9395f324cd2e7f331f":\n      credit-limit: 2000\n    "fink_max_3d6be4e0c6f57d845c49b853b246053a":\n      credit-limit: 200000'
)

with open('c:/gitroot/backup/config.yaml', 'w', encoding='utf-8') as f:
    f.write(text)

with open('c:/gitroot/backup/pgstore/config/config.yaml', 'w', encoding='utf-8') as f:
    f.write(text)

print('Updated both configs successfully.')
