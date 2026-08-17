import type { NextRequest } from "next/server";

export const dynamic = "force-dynamic";

/**
 * Legacy static-file proxy. It never reads local files directly and requires
 * the same bearer token as the contract download API.
 */
export async function GET(
    request: NextRequest,
    context: { params: Promise<{ path: string[] }> }
) {
    const base = process.env.NEXT_PUBLIC_DEV_URL || "";
    const { path } = await context.params;
    
    // 构建后端静态文件 URL
    const staticPath = path.join("/");
    const upstreamUrl = `${base}/api/static/${staticPath}`;
    
    const auth = request.headers.get("authorization");
    if (!auth) {
        return new Response("Unauthorized", {
            status: 401,
            headers: {
                "cache-control": "private, no-store",
            },
        });
    }

    try {
        const upstream = await fetch(upstreamUrl, {
            method: "GET",
            headers: { Authorization: auth },
            signal: request.signal,
        });

        if (!upstream.ok) {
            return new Response(upstream.body, {
                status: upstream.status,
                statusText: upstream.statusText,
                headers: {
                    "cache-control": "private, no-store",
                },
            });
        }

        // 复制响应头
        const headers = new Headers();
        const contentType = upstream.headers.get("content-type");
        const contentDisposition = upstream.headers.get("content-disposition");
        const contentLength = upstream.headers.get("content-length");
        
        if (contentType) headers.set("content-type", contentType);
        if (contentDisposition) headers.set("content-disposition", contentDisposition);
        if (contentLength) headers.set("content-length", contentLength);
        
        headers.set("cache-control", "private, no-store");

        return new Response(upstream.body, {
            status: upstream.status,
            statusText: upstream.statusText,
            headers,
        });
    } catch (error) {
        console.error("Static file proxy error:", error);
        return new Response("Failed to fetch static file", {
            status: 500,
            headers: {
                "cache-control": "private, no-store",
            },
        });
    }
}
