/**
 * 403 错误统一处理模块
 * 用于在接口返回 403 状态时显示重新登录的模态框
 * 确保同一时间只弹出一个模态框，不会重复弹出
 */

type Auth403Listener = () => void;
type LoginCallback = () => void;

class AuthDatedHandler {
    private listeners: Set<Auth403Listener> = new Set();
    private isModalShowing = false;
    private loginCallback: LoginCallback | null = null;

    /**
     * 订阅 403 错误事件
     * @param listener 监听器函数
     * @returns 取消订阅的函数
     */
    subscribe(listener: Auth403Listener): () => void {
        this.listeners.add(listener);
        return () => {
            this.listeners.delete(listener);
        };
    }

    /**
     * 注册登录回调函数
     * @param callback 登录回调函数
     * @returns 取消注册的函数
     */
    registerLoginCallback(callback: LoginCallback): () => void {
        this.loginCallback = callback;
        return () => {
            this.loginCallback = null;
        };
    }

    /**
     * 触发登录模态框
     */
    triggerLogin(): void {
        if (this.loginCallback) {
            this.loginCallback();
        }
    }

    /**
     * 触发 403 错误事件
     * 确保同一时间只触发一次，防止重复弹出模态框
     */
    trigger403Error(): void {
        // 如果模态框已经在显示，不再重复触发
        if (this.isModalShowing) {
            return;
        }

        this.isModalShowing = true;
        this.listeners.forEach((listener) => {
            try {
                listener();
            } catch (error) {
                console.error('authDatedHandler listener error:', error);
            }
        });
    }

    /**
     * 关闭模态框时调用，重置状态
     */
    closeModal(): void {
        this.isModalShowing = false;
    }

    /**
     * 获取模态框是否正在显示
     */
    isShowing(): boolean {
        return this.isModalShowing;
    }
}

// 创建单例实例
export const authDatedHandler = new AuthDatedHandler();
