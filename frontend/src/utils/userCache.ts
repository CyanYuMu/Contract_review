/**
 * 用户信息缓存管理模块
 * 使用 sessionStorage 缓存用户信息，避免重复请求 /user/me 接口
 * sessionStorage 在页面刷新后仍保留，但关闭浏览器后会清除
 */

import type { User } from '@/lib/Interface';

const USER_CACHE_KEY = 'user_info_cache';

/**
 * 获取缓存的用户信息
 */
export function getCachedUser(): User | null {
    if (typeof window === 'undefined') return null;
    
    try {
        const cached = sessionStorage.getItem(USER_CACHE_KEY);
        if (cached) {
            return JSON.parse(cached) as User;
        }
    } catch (error) {
        console.warn('读取用户缓存失败:', error);
    }
    return null;
}

/**
 * 设置用户信息缓存
 */
export function setCachedUser(user: User): void {
    if (typeof window === 'undefined') return;
    
    try {
        sessionStorage.setItem(USER_CACHE_KEY, JSON.stringify(user));
    } catch (error) {
        console.warn('设置用户缓存失败:', error);
    }
}

/**
 * 清除用户信息缓存
 */
export function clearCachedUser(): void {
    if (typeof window === 'undefined') return;
    
    try {
        sessionStorage.removeItem(USER_CACHE_KEY);
    } catch (error) {
        console.warn('清除用户缓存失败:', error);
    }
}

/**
 * 检查是否有有效的用户缓存
 */
export function hasCachedUser(): boolean {
    return getCachedUser() !== null;
}
