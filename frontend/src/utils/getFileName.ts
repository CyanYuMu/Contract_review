export function getFileName(title: string): { left: string; right: string } | null {
    const rest = title.split(/[:：]\s*/).at(-1)?.trim();
    if (!rest) return null;

    const parts = rest.split(/\s+vs\s+/i).map(s => s.trim()).filter(Boolean);
    if (parts.length !== 2) return null;

    const [left, right] = parts;
    return { left, right };
}
