import {NextRequest, NextResponse} from 'next/server';

export async function GET(
    request: NextRequest,
    context: { params: Promise<{ path: string[] }> }
) {
    const {path} = await context.params;
    return handleRequest(request, path, 'GET');
}

export async function POST(
    request: NextRequest,
    context: { params: Promise<{ path: string[] }> }
) {
    const { path } = await context.params;
    // SSE直通：当客户端声明接收SSE或特定流式路径时，走流式转发
    const accept = request.headers.get('accept') || '';
    const joinedPath = path.join('/');
    if (accept.includes('text/event-stream') || joinedPath === 'chat' || joinedPath === 'chat/chat' || joinedPath === 'review_task/start_task' || joinedPath === 'qa/ask') {
        return handleChatStreamRequest(request, path);
    }
    return handleRequest(request, path, 'POST');
}

export async function PUT(
    request: NextRequest,
    context: { params: Promise<{ path: string[] }> }
) {
    const {path} = await context.params;
    return handleRequest(request, path, 'PUT');
}

export async function DELETE(
    request: NextRequest,
    context: { params: Promise<{ path: string[] }> }
) {
    const {path} = await context.params;
    return handleRequest(request, path, 'DELETE');
}

// ============ SSE 流式处理函数 ============
async function handleChatStreamRequest(
    request: NextRequest,
    path: string[]
) {
    const backendUrl = process.env.NEXT_PUBLIC_DEV_URL;
    if (!backendUrl) {
        return NextResponse.json(
            {error: 'Proxy request failed', details: 'NEXT_PUBLIC_DEV_URL is not configured'},
            {status: 500}
        );
    }
    const targetPath = path.join('/');
    const targetUrl = `${backendUrl}/api/${targetPath}`;

    console.log('[SSE] 流式请求:', {
        backendUrl,
        targetPath,
        targetUrl,
    });

    const body = await request.text();

    const headers: Record<string, string> = {
        'Content-Type': 'application/json',
    };

    const authHeader = request.headers.get('Authorization');
    if (authHeader) {
        headers['Authorization'] = authHeader;
    }

    try {
        const response = await fetch(targetUrl, {
            method: 'POST',
            headers,
            body,
            signal: request.signal,
        });

        if (!response.ok) {
            const errorData = await response.json().catch(() => ({error: 'Request failed'}));
            console.error('[SSE] 请求失败:', errorData);
            return NextResponse.json(errorData, {status: response.status});
        }

        return new Response(response.body, {
            headers: {
                'Content-Type': 'text/event-stream',
                'Cache-Control': 'no-cache',
                'Connection': 'keep-alive',
                'Access-Control-Allow-Origin': '*',
            },
        });
    } catch (error) {
        const errorMessage = error instanceof Error ? error.message : 'Unknown error';
        return NextResponse.json(
            {error: 'SSE request failed', details: errorMessage},
            {status: 500}
        );
    }
}

async function handleRequest(
    request: NextRequest,
    path: string[],
    method: string
) {
    const backendUrl = process.env.NEXT_PUBLIC_DEV_URL;
    const targetPath = path.join('/');
    const search = request.nextUrl.search || '';
    const targetUrl = `${backendUrl}/api/${targetPath}${search}`;


    const contentType = request.headers.get('content-type') || '';
    const isMultipart = contentType.includes('multipart/form-data');

    let body: FormData | string | undefined;
    const headers: Record<string, string> = {};

    if (method !== 'GET') {
        if (isMultipart) {
            body = await request.formData();
        } else {
            body = await request.text();
            headers['Content-Type'] = 'application/json';
        }
    }

    const authHeader = request.headers.get('Authorization');
    const cookieHeader = request.headers.get('Cookie');

    if (authHeader) {
        headers['Authorization'] = authHeader;
    }
    if (cookieHeader) {
        headers['Cookie'] = cookieHeader;
    }

    try {
        const response = await fetch(targetUrl, {
            method,
            headers,
            body,
            signal: request.signal,
        });

        const contentType = response.headers.get('content-type') || '';
        const isJsonExpected = contentType.includes('application/json');
        const commonHeaders = {
            'Access-Control-Allow-Origin': '*',
            'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
            'Access-Control-Allow-Headers': 'Content-Type, Authorization',
        };

        // Preserve binary responses as streams. Reading them as text corrupts
        // DOCX and PDF downloads.
        if (!isJsonExpected) {
            const responseHeaders = new Headers(commonHeaders);
            for (const name of ['content-type', 'content-disposition', 'content-length', 'accept-ranges']) {
                const value = response.headers.get(name);
                if (value) responseHeaders.set(name, value);
            }
            responseHeaders.set('cache-control', 'private, no-store');
            return new NextResponse(response.body, {
                status: response.status,
                headers: responseHeaders,
            });
        }

        let rawText: string | null = null;
        let data: unknown = null;
        let isJson: boolean = isJsonExpected;

        try {
            rawText = await response.text();
            if (isJsonExpected) {
                try {
                    data = rawText ? JSON.parse(rawText) : null;
                } catch {
                    data = rawText;
                    isJson = false;
                }
            } else {
                data = rawText;
            }
        } catch (readErr) {
            return NextResponse.json(
                {
                    error: 'Proxy request failed',
                    details: readErr instanceof Error ? readErr.message : 'Unknown error when reading response',
                    targetUrl,
                    status: response.status,
                },
                {status: 500}
            );
        }

        console.log('代理响应:', {
            status: response.status,
            data: isJson ? data : '[non-json body]',
        });

        if (isJson) {
            return NextResponse.json(data, {
                status: response.status,
                headers: commonHeaders,
            });
        }

        return new NextResponse((data ?? '').toString(), {
            status: response.status,
            headers: {
                ...commonHeaders,
                'Content-Type': contentType || 'text/plain',
            },
        });
    } catch (error) {
        console.error('代理请求失败:', error);
        const errorMessage = error instanceof Error ? error.message : 'Unknown error';
        return NextResponse.json(
            {error: 'Proxy request failed', details: errorMessage},
            {status: 500}
        );
    }
}
