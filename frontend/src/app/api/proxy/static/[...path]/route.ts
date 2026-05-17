import type { NextRequest } from "next/server";
import { readFile } from "node:fs/promises";
import path from "node:path";

export const dynamic = "force-dynamic";

/**
 * 静态文件代理路由
 * 将 /api/proxy/static/xxx 的请求代理到后端的 /static/xxx
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

    try {
        const upstream = await fetch(upstreamUrl, {
            method: "GET",
            headers: auth ? { Authorization: auth } : undefined,
        });

        if (!upstream.ok) {
            return serveLocalUploadFallback(staticPath, upstream.status, upstream.statusText);
        }

        // 复制响应头
        const headers = new Headers();
        const contentType = upstream.headers.get("content-type");
        const contentDisposition = upstream.headers.get("content-disposition");
        const contentLength = upstream.headers.get("content-length");
        
        if (contentType) headers.set("content-type", contentType);
        if (contentDisposition) headers.set("content-disposition", contentDisposition);
        if (contentLength) headers.set("content-length", contentLength);
        
        // 设置缓存头
        headers.set("cache-control", "public, max-age=31536000, immutable");

        return new Response(upstream.body, {
            status: upstream.status,
            statusText: upstream.statusText,
            headers,
        });
    } catch (error) {
        console.error("Static file proxy error:", error);
        return serveLocalUploadFallback(staticPath, 500, "Failed to fetch static file");
    }
}

async function serveLocalUploadFallback(
    staticPath: string,
    status: number,
    statusText: string
) {
    const fileName = decodeURIComponent(path.basename(staticPath));
    const candidates = [
        path.resolve(process.cwd(), "../app/uploads", fileName),
        path.resolve(process.cwd(), "app/uploads", fileName),
        path.resolve(process.cwd(), "uploads", fileName),
    ];

    for (const candidate of candidates) {
        try {
            const file = await readFile(candidate);
            return new Response(new Uint8Array(file), {
                status: 200,
                headers: {
                    "content-type": contentTypeFor(fileName),
                    "cache-control": "public, max-age=31536000, immutable",
                },
            });
        } catch {
        }
    }

    return new Response(`File not found: ${staticPath}`, {
        status,
        statusText,
    });
}

function contentTypeFor(fileName: string) {
    const ext = path.extname(fileName).toLowerCase();
    if (ext === ".docx") {
        return "application/vnd.openxmlformats-officedocument.wordprocessingml.document";
    }
    if (ext === ".pdf") {
        return "application/pdf";
    }
    if (ext === ".txt") {
        return "text/plain; charset=utf-8";
    }
    return "application/octet-stream";
}
