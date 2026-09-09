import 'package:flutter/foundation.dart';

import '../core/api.dart';
import '../core/models.dart';

/// 上传任务状态
enum UploadStatus { waiting, hashing, checking, uploading, merging, done, failed }

/// 上传任务
class UploadTask {
  final String path;
  final String name;
  final int size;
  UploadStatus status;
  double progress;
  String? error;

  UploadTask({
    required this.path,
    required this.name,
    required this.size,
    this.status = UploadStatus.waiting,
    this.progress = 0,
  });

  bool get isFinished => status == UploadStatus.done || status == UploadStatus.failed;
}

/// 面包屑节点
class Crumb {
  final int id;
  final String name;
  const Crumb(this.id, this.name);
}

/// 全局应用状态
class AppState extends ChangeNotifier {
  bool initialized = false;
  bool loggedIn = false;

  User? user;
  String? token;

  // 文件浏览状态
  final List<Crumb> crumbs = [const Crumb(0, '全部文件')];
  List<FileItem> files = [];
  bool loading = false;
  bool gridMode = false;
  String? error;

  // 空间配额
  int quotaMax = 0;
  int quotaUsed = 0;

  // 上传任务
  final List<UploadTask> uploads = [];

  /// 初始化：恢复会话
  Future<void> init() async {
    final savedToken = await Session.loadToken();
    final savedUser = await Session.loadUser();
    if (savedToken != null && savedUser != null) {
      token = savedToken;
      ApiClient.instance.token = savedToken;
      user = User.fromJson(savedUser);
      loggedIn = true;
      // 刷新用户信息，失败则视为会话过期
      try {
        await refreshUser();
      } catch (_) {
        await logout();
      }
    }
    initialized = true;
    notifyListeners();
  }

  /// 登录
  Future<void> login(String username, String password) async {
    final data = await ApiClient.instance
        .post('/auth/login', {'username': username, 'password': password});
    await _applyAuth(data);
  }

  /// 注册
  Future<void> register(String username, String password, String email) async {
    final data = await ApiClient.instance.post('/auth/register', {
      'username': username,
      'password': password,
      'email': email,
    });
    await _applyAuth(data);
  }

  Future<void> _applyAuth(Map<String, dynamic> data) async {
    final t = (data['token'] as String?) ?? '';
    final u = data['user'];
    if (u is! Map<String, dynamic>) {
      throw const ApiException(-1, '登录响应缺少用户信息');
    }
    token = t;
    ApiClient.instance.token = t;
    user = User.fromJson(u);
    await Session.save(t, u);
    loggedIn = true;
    notifyListeners();
  }

  /// 获取当前用户信息
  Future<void> refreshUser() async {
    final data = await ApiClient.instance.get('/auth/me');
    final u = data['user'];
    if (u is Map<String, dynamic>) {
      user = User.fromJson(u);
      notifyListeners();
    }
  }

  /// 退出登录
  Future<void> logout() async {
    token = null;
    ApiClient.instance.token = null;
    user = null;
    loggedIn = false;
    files = [];
    uploads.clear();
    crumbs
      ..clear()
      ..add(const Crumb(0, '全部文件'));
    await Session.clear();
    notifyListeners();
  }

  /// 当前目录
  int get currentParentId => crumbs.last.id;

  /// 切换视图模式
  void setGridMode(bool value) {
    gridMode = value;
    notifyListeners();
  }

  /// 供上传服务等外部刷新状态（上传进度变化）
  void refresh() => notifyListeners();

  /// 进入文件夹
  Future<void> enterFolder(FileItem folder) async {
    crumbs.add(Crumb(folder.id, folder.name));
    await loadFiles();
  }

  /// 面包屑跳转
  Future<void> jumpTo(int index) async {
    if (index < 0 || index >= crumbs.length) return;
    crumbs.removeRange(index + 1, crumbs.length);
    await loadFiles();
  }

  /// 回到上级
  Future<void> goBack() async {
    if (crumbs.length > 1) {
      crumbs.removeLast();
      await loadFiles();
    }
  }

  /// 刷新文件列表 + 配额
  Future<void> loadFiles() async {
    loading = true;
    error = null;
    notifyListeners();
    try {
      final data = await ApiClient.instance.get('/files/list', query: {
        'parent_id': currentParentId,
        'page': 1,
        'page_size': 100,
      });
      final list = data['list'] ?? data['files'] ?? data['items'] ?? [];
      files = list
          .whereType<Map<String, dynamic>>()
          .map(FileItem.fromJson)
          .toList();
    } catch (e) {
      error = e.toString();
    } finally {
      loading = false;
      notifyListeners();
    }
  }

  /// 加载配额
  Future<void> loadQuota() async {
    try {
      final data = await ApiClient.instance.get('/files/quota');
      quotaMax = (data['quota_max'] as num?)?.toInt() ?? quotaMax;
      quotaUsed = (data['quota_used'] as num?)?.toInt() ?? quotaUsed;
      notifyListeners();
    } catch (_) {
      // 配额失败不影响主流程
    }
  }

  // ---------- 文件操作 ----------

  Future<void> mkdir(String name) async {
    await ApiClient.instance
        .post('/files/mkdir', {'parent_id': currentParentId, 'name': name});
    await loadFiles();
  }

  Future<void> rename(FileItem file, String newName) async {
    await ApiClient.instance
        .post('/files/rename', {'file_id': file.id, 'new_name': newName});
    await loadFiles();
  }

  Future<void> move(FileItem file, int targetParentId) async {
    await ApiClient.instance.post('/files/move', {
      'file_id': file.id,
      'target_parent_id': targetParentId,
    });
    await loadFiles();
  }

  Future<void> delete(List<FileItem> items) async {
    await ApiClient.instance
        .post('/files/delete', {'file_ids': items.map((f) => f.id).toList()});
    await loadFiles();
    await loadQuota();
  }

  // ---------- 分享 ----------

  Future<ShareInfo> createShare(FileItem file, {String? password, String? expireAt}) async {
    final data = await ApiClient.instance.post('/share/create', {
      'file_id': file.id,
      if (password != null && password.isNotEmpty) 'password': password,
      if (expireAt != null && expireAt.isNotEmpty) 'expire_at': expireAt,
    });
    return ShareInfo.fromJson(data);
  }

  // ---------- 上传任务管理 ----------

  void addUpload(UploadTask task) {
    uploads.add(task);
    notifyListeners();
  }

  void removeUpload(UploadTask task) {
    uploads.remove(task);
    notifyListeners();
  }

  void clearUploads() {
    uploads.removeWhere((t) => t.status == UploadStatus.done || t.status == UploadStatus.failed);
    notifyListeners();
  }
}
