import fs from "node:fs";
import { fileURLToPath } from "node:url";
import { parsers } from "../web/node_modules/prettier/plugins/typescript.mjs";

const root = fileURLToPath(new URL("../web/src/", import.meta.url));
const missing = [];
const counts = { functions: 0, types: 0, fields: 0 };

// checkFile verifies descriptions on named functions, data types, and declared fields.
async function checkFile(file) {
  const source = fs.readFileSync(root + file, "utf8");
  const ast = await parsers.typescript.parse(source, { filepath: file });
  const comments = ast.comments || [];

  // requireDescription accepts an adjacent nonempty comment before the documented declaration.
  function requireDescription(node, name) {
    const described = comments.some(
      (comment) =>
        comment.value.trim() &&
        comment.range[1] <= node.range[0] &&
        source.slice(comment.range[1], node.range[0]).trim() === "",
    );
    if (!described)
      missing.push(
        `${file}:${node.loc.start.line}: ${name} needs a description`,
      );
  }

  // walk checks declarations while ignoring parser metadata and inline rendering callbacks.
  function walk(node, parent, grandparent) {
    if (!node || typeof node !== "object") return;
    if (node.type === "FunctionDeclaration") {
      counts.functions++;
      requireDescription(
        parent?.type === "ExportNamedDeclaration" ? parent : node,
        node.id.name,
      );
    }
    if (
      node.type === "VariableDeclarator" &&
      node.id.type === "Identifier" &&
      (node.init?.type === "ArrowFunctionExpression" ||
        node.init?.type === "FunctionExpression" ||
        node.init?.callee?.name === "useCallback")
    ) {
      counts.functions++;
      requireDescription(
        grandparent?.type === "ExportNamedDeclaration" ? grandparent : parent,
        node.id.name,
      );
    }
    if (
      ["TSTypeAliasDeclaration", "TSInterfaceDeclaration"].includes(node.type)
    ) {
      counts.types++;
      requireDescription(
        parent?.type === "ExportNamedDeclaration" ? parent : node,
        node.id.name,
      );
    }
    if (node.type === "TSPropertySignature") {
      counts.fields++;
      requireDescription(node, node.key.name);
    }
    for (const [key, value] of Object.entries(node)) {
      if (["loc", "range", "comments", "tokens"].includes(key)) continue;
      if (Array.isArray(value))
        value.forEach((child) => walk(child, node, parent));
      else if (value && typeof value === "object") walk(value, node, parent);
    }
  }
  walk(ast);
}

const files = fs
  .readdirSync(root, { recursive: true })
  .filter(
    (file) =>
      /\.tsx?$/.test(file) &&
      !file.includes(".test.") &&
      !file.endsWith(".d.ts"),
  );
for (const file of files) await checkFile(file);
if (missing.length) {
  console.error(missing.join("\n"));
  process.exitCode = 1;
} else {
  console.log(
    `Documented ${counts.functions} named functions, ${counts.types} types, and ${counts.fields} fields.`,
  );
}
