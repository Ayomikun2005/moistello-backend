class RedisService {
  constructor() {
    this.client = null;
  }
  isAlive() {
    return this.client !== null;
  }
  async connect(factoryOrClient) {
    if (typeof factoryOrClient === 'function') {
      try {
        this.client = await factoryOrClient();
      } catch (err) {
        this.client = null;
        throw new Error(`RedisService.connect: ${err.message}`);
      }
    } else {
      this.client = factoryOrClient;
    }
  }
  async disconnect() {
    if (this.client && typeof this.client.quit === 'function') {
      await this.client.quit();
    }
    this.client = null;
  }
  async get(key) {
    if (!this.client) throw new Error('RedisService.get: not connected');
    return this.client.get(key);
  }
  async set(key, value, ttl) {
    if (!this.client) throw new Error('RedisService.set: not connected');
    if (ttl !== undefined) {
      return this.client.set(key, value, 'EX', ttl);
    }
    return this.client.set(key, value);
  }
  async del(...keys) {
    if (!this.client) throw new Error('RedisService.del: not connected');
    return this.client.del(...keys);
  }
}

const redisService = new RedisService();
export default redisService;
