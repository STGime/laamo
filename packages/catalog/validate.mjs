// Validates bricks.json and templates/*.json against their JSON schemas.
// Run by CI (pnpm --filter @laamo/catalog test). Semantic checks (references,
// max 3 visible settings, templates validating as stacks) are Go tests in
// backend/internal/catalog.
import { readFileSync, readdirSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import Ajv2020 from 'ajv/dist/2020.js';

const here = dirname(fileURLToPath(import.meta.url));
const read = (p) => JSON.parse(readFileSync(join(here, p), 'utf8'));

const ajv = new Ajv2020({ allErrors: true, strict: true, allowUnionTypes: true, strictRequired: false });
ajv.addSchema(read('bricks.schema.json'), 'bricks.schema.json');
ajv.addSchema(read('template.schema.json'), 'template.schema.json');

let failed = false;
const check = (schema, file) => {
	const ok = ajv.validate(schema, read(file));
	if (!ok) {
		failed = true;
		console.error(`✗ ${file}`);
		for (const e of ajv.errors) console.error(`  ${e.instancePath || '/'} ${e.message}`);
	} else {
		console.log(`✓ ${file}`);
	}
};

check('bricks.schema.json', 'bricks.json');
for (const f of readdirSync(join(here, 'templates')).filter((f) => f.endsWith('.json'))) {
	check('template.schema.json', `templates/${f}`);
}
process.exit(failed ? 1 : 0);
