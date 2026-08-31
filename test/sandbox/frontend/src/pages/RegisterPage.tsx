import { useState } from 'react';
import { useNavigate, Link } from 'react-router-dom';
import { AuthForm } from '../components/AuthForm';
import { createV1AuthSignup } from '../lib/api';

interface CreateV1AuthSignupResult {
  token: string;
}

export function RegisterPage() {
  const navigate = useNavigate();
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (username: string, email: string | undefined, password: string) => {
    setIsSubmitting(true);
    try {
      const result = await createV1AuthSignup({ username, email, password });
      if (result.isOk && result.token) {
        localStorage.setItem('token', result.token);
        navigate('/feed');
      } else {
        alert('Registration failed');
      }
    } catch (err: any) {
      alert(err.message || 'Registration failed');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50 p-4">
      <div className="max-w-md w-full bg-white rounded-xl shadow-lg p-8">
        <h1 className="text-2xl font-bold text-gray-900 mb-2 text-center">Create Account</h1>
        <p className="text-gray-600 text-center mb-6">Join us and start sharing your posts</p>
        <AuthForm onSubmit={handleSubmit} onReset={() => {}} />
        <div className="mt-6 text-center">
          <p className="text-gray-600">Already have an account? <Link to="/login" className="text-blue-600 hover:underline">Sign in</Link></p>
        </div>
      </div>
    </div>
  );
}
