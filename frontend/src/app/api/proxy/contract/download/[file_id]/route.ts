import type { NextRequest } from "next/server";

export const dynamic = "force-dynamic";

const pickHeaders = (headers: Headers) => {
    const next = new Headers();
    const contentType = headers.get("content-type");
    const contentDisposition = headers.get("content-disposition");
    const contentLength = headers.get("content-length");
    if (contentType) next.set("content-type", contentType);
    if (contentDisposition) next.set("content-disposition", contentDisposition);
    if (contentLength) next.set("content-length", contentLength);
    return next;
};

export async function GET(
    request: NextRequest,
    context: { params: Promise<{ file_id: string }> }
) {
    const base = process.env.NEXT_PUBLIC_DEV_URL || "";
    const { file_id } = await context.params;
    const upstreamUrl = `${base}/api/contract/download/${file_id}`;
    const auth = request.headers.get("authorization");

    const upstream = await fetch(upstreamUrl, {
        method: "GET",
        headers: auth ? { Authorization: auth } : undefined,
    });

    return new Response(upstream.body, {
        status: upstream.status,
        statusText: upstream.statusText,
        headers: pickHeaders(upstream.headers),
    });
}
