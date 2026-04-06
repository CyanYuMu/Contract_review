import { useEffect, useState } from 'react';
import { getUserInfo } from '@/lib/api/user';

/**
 * 权限类型定义
 */
export type PermissionKey = 
  // 合同审阅权限
  | 'contractReview'
  | 'riskIdentification'
  | 'contractModification'
  | 'contractExport'
  | 'contractSwitching'
  // 合同比对权限
  | 'contractComparison'
  | 'riskClassification'
  | 'contractExportAnnotation'
  | 'contractSwitchingStatus'
  // 智审记录权限
  | 'auditRecordSelf'
  | 'auditRecordDepartment'
  | 'auditRecordPlatform'
  | 'auditRecordOthers'
  // 数据看板权限
  | 'dataBoard';

/**
 * 用户权限信息接口
 */
interface UserPermissions {
  [key: string]: boolean;
}

/**
 * 权限判断 Hook
 * @param permission - 需要判断的权限 key 或权限 key 数组
 * @param requireAll - 当传入数组时，是否需要全部权限都满足（默认 false，即满足任一权限即可）
 * @returns 是否有权限
 */
export function usePermission(
  permission: PermissionKey | PermissionKey[],
  requireAll: boolean = false
): boolean {
  const [hasPermission, setHasPermission] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(true);

  useEffect(() => {
    const checkPermission = async () => {
      try {
        setLoading(true);
        
        // 获取用户信息（包含权限信息）
        const userInfo = await getUserInfo();
        
        // TODO: 根据实际 API 返回结构调整
        // 假设 userInfo 中包含 permissions 字段
        const userPermissions: UserPermissions = userInfo.permissions || {};

        // 判断权限
        if (Array.isArray(permission)) {
          // 数组权限判断
          if (requireAll) {
            // 需要全部权限
            setHasPermission(
              permission.every(key => userPermissions[key] === true)
            );
          } else {
            // 满足任一权限即可
            setHasPermission(
              permission.some(key => userPermissions[key] === true)
            );
          }
        } else {
          // 单个权限判断
          setHasPermission(userPermissions[permission] === true);
        }
      } catch (error) {
        console.error('权限检查失败:', error);
        setHasPermission(false);
      } finally {
        setLoading(false);
      }
    };

    checkPermission();
  }, [permission, requireAll]);

  return hasPermission;
}

/**
 * 批量权限判断 Hook
 * @returns 权限判断函数和加载状态
 */
export function usePermissions() {
  const [permissions, setPermissions] = useState<UserPermissions>({});
  const [loading, setLoading] = useState<boolean>(true);

  useEffect(() => {
    const fetchPermissions = async () => {
      try {
        setLoading(true);
        const userInfo = await getUserInfo();
        setPermissions(userInfo.permissions || {});
      } catch (error) {
        console.error('获取权限失败:', error);
        setPermissions({});
      } finally {
        setLoading(false);
      }
    };

    fetchPermissions();
  }, []);

  /**
   * 检查单个权限
   */
  const hasPermission = (key: PermissionKey): boolean => {
    return permissions[key] === true;
  };

  /**
   * 检查多个权限（满足任一即可）
   */
  const hasAnyPermission = (keys: PermissionKey[]): boolean => {
    return keys.some(key => permissions[key] === true);
  };

  /**
   * 检查多个权限（需要全部满足）
   */
  const hasAllPermissions = (keys: PermissionKey[]): boolean => {
    return keys.every(key => permissions[key] === true);
  };

  return {
    permissions,
    loading,
    hasPermission,
    hasAnyPermission,
    hasAllPermissions
  };
}