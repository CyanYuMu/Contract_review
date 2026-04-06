export function replace(
    original: string,
    suggested: string
): { result: string; replaced: boolean } {
    const escJson = (s: string) =>
        s.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\r/g, '\\r').replace(/\n/g, '\\n');

    try {
        const obj = JSON.parse(original);
        if (obj && typeof obj === 'object' && Object.prototype.hasOwnProperty.call(obj, 'source_content')) {
            obj.source_content = suggested;
            return {result: JSON.stringify(obj, null, 2), replaced: true};
        }
    } catch {

    }


    let out = original.replace(
        /("source_content"\s*:\s*)"((?:\\.|[^"\\])*)"/,
        (_m, p1) => `${p1}"${escJson(suggested)}"`
    );
    if (out !== original) return {result: out, replaced: true};

    out = original.replace(
        /('source_content'\s*:\s*)'((?:\\.|[^'\\])*)'/,
        (_m, p1) => {
            const esc = suggested.replace(/'/g, "\\'");
            return `${p1}'${esc}'`;
        }
    );
    if (out !== original) return {result: out, replaced: true};

    return {result: original, replaced: false};
}

export default replace;
