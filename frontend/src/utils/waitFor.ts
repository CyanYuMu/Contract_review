/**
 * waitFor 轮询等待条件成立，集中替代散落的 while + setTimeout 盲等模式。
 *
 * @param predicate 返回 truthy 时结束等待
 * @param options.timeout 总超时（ms），超时后返回最后一次的 falsy 值，默认 5000
 * @param options.interval 轮询间隔（ms），默认 50
 * @returns predicate 最终返回值（超时则为最后一次的 falsy 值）
 */
export async function waitFor<T>(
    predicate: () => T,
    options: { timeout?: number; interval?: number } = {}
): Promise<T> {
    const { timeout = 5000, interval = 50 } = options;
    const deadline = Date.now() + timeout;
    let result: T;
    // eslint-disable-next-line no-cond-assign
    while (!(result = predicate())) {
        if (Date.now() >= deadline) {
            return result;
        }
        await new Promise((resolve) => setTimeout(resolve, interval));
    }
    return result;
}
